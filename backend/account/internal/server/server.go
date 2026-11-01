// Package server assembles accountd's HTTP surface: an API/OIDC side and a
// reverse-proxied frontend side, behind one listener (the single front door).
package server

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"account/backend/account/internal/handler"
	"account/backend/account/internal/mailer"
	"account/backend/account/internal/repository"
	"account/backend/account/internal/service"
	"account/backend/account/internal/session"
)

// Deps are the infrastructure handles the router needs.
type Deps struct {
	FrontendURL string
	Pool        *pgxpool.Pool
	RDB         *redis.Client
	Secure      bool
	SMTP        mailer.Config
	Log         *slog.Logger
}

// New builds the root handler. FrontendURL may be empty, in which case
// non-API routes return 404 (API-only mode).
func New(d Deps) (http.Handler, error) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// --- domain wiring -------------------------------------------------------
	accounts := repository.NewAccount(d.Pool)
	personaRepo := repository.NewPersona(d.Pool)
	recoveryRepo := repository.NewRecoveryCode(d.Pool)
	claimRepo := repository.NewIdentityClaim(d.Pool)
	notifyRepo := repository.NewNotification(d.Pool)
	svc := service.NewAccount(accounts, personaRepo, recoveryRepo, claimRepo, notifyRepo)
	sessions := session.New(d.RDB, d.Secure)
	mail := mailer.New(d.SMTP, d.Log)

	authH := handler.NewAuth(svc, sessions, d.Log)
	personaH := handler.NewPersona(svc, sessions, d.Log)
	identityH := handler.NewIdentity(svc, sessions, mail, d.Log)
	notifyH := handler.NewNotify(notifyRepo, sessions, d.Log)
	limiter := handler.NewRateLimiter(d.RDB)

	// --- health --------------------------------------------------------------
	r.Get("/healthz", handler.Healthz)

	// --- API -----------------------------------------------------------------
	r.Route("/api", func(api chi.Router) {
		api.With(limiter.Protect("register", 5, time.Hour)).Post("/auth/register", authH.Register)
		api.With(limiter.Protect("login", 10, 15*time.Minute)).Post("/auth/login", authH.Login)
		api.Post("/auth/logout", authH.Logout)
		api.With(limiter.Protect("forgot", 10, time.Hour)).Post("/auth/forgot-password", authH.ForgotPassword)
		api.With(limiter.Protect("reset", 10, time.Hour)).Post("/auth/reset-password", authH.ResetPassword)

		api.Get("/me", authH.Me)
		api.Patch("/me", authH.ChangeUsername)
		api.With(limiter.Protect("password", 10, time.Hour)).Post("/me/password", authH.ChangePassword)
		api.Post("/me/recovery-codes", authH.RegenerateRecoveryCodes)

		api.Get("/personas", personaH.List)
		api.Post("/personas", personaH.Create)
		api.Patch("/personas/{id}", personaH.Update)
		api.With(limiter.Protect("identity-otp", 10, time.Hour)).Post("/personas/{id}/identity/verify", identityH.Request)
		api.Post("/personas/{id}/identity/confirm", identityH.Confirm)

		api.Get("/notifications", notifyH.List)
		// Static path before {id}/read so "unread-count" isn't parsed as a UUID.
		api.Get("/notifications/unread-count", notifyH.UnreadCount)
		api.Post("/notifications/{id}/read", notifyH.MarkRead)
		api.Post("/notifications/read-all", notifyH.MarkAllRead)
	})

	// --- Frontend (everything else) ----------------------------------------
	if d.FrontendURL != "" {
		proxy, err := newFrontendProxy(d.FrontendURL)
		if err != nil {
			return nil, err
		}
		r.Handle("/*", proxy)
	} else {
		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			http.NotFound(w, req)
		})
	}
	return r, nil
}

// newFrontendProxy reverse-proxies non-API routes to the SvelteKit server
// (Vite dev server in dev, built Node server in prod).
func newFrontendProxy(target string) (http.Handler, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	p := httputil.NewSingleHostReverseProxy(u)
	original := p.Director
	p.Director = func(r *http.Request) {
		original(r)
		r.Host = u.Host
	}
	return p, nil
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
			)
		})
	}
}

package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/ory/fosite"

	"account/backend/account/internal/model"
	"account/backend/account/internal/oidc"
	"account/backend/account/internal/repository"
	"account/backend/account/internal/service"
	"account/backend/account/internal/session"
)

// OIDC serves the standards endpoints products use to authenticate.
type OIDC struct {
	provider fosite.OAuth2Provider
	keys     *oidc.KeyService
	personas *repository.Persona
	sessions *session.Store
	svc      *service.Account
	defaults *oidc.PersonaDefault
	issuer   string
	log      *slog.Logger
}

// NewOIDC wires the handler.
func NewOIDC(
	provider fosite.OAuth2Provider,
	keys *oidc.KeyService,
	personas *repository.Persona,
	sessions *session.Store,
	svc *service.Account,
	defaults *oidc.PersonaDefault,
	issuer string,
	log *slog.Logger,
) *OIDC {
	return &OIDC{
		provider: provider,
		keys:     keys,
		personas: personas,
		sessions: sessions,
		svc:      svc,
		defaults: defaults,
		issuer:   issuer,
		log:      log,
	}
}

// ---- GET /authorize ----

// Authorize runs the authorization-code flow's front channel:
// login session → persona selection → code issuance redirect.
func (h *OIDC) Authorize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse + validate the request (client, redirect_uri, PKCE, scopes).
	areq, err := h.provider.NewAuthorizeRequest(ctx, r)
	if err != nil {
		h.provider.WriteAuthorizeError(ctx, w, areq, err)
		return
	}

	// Login session required.
	acctID, err := h.sessions.Account(ctx, r)
	if err != nil {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	acct, err := h.svc.Get(ctx, acctID)
	if err != nil {
		h.log.Error("authorize: get account", "err", err)
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	personas, err := h.svc.ListPersonas(ctx, acctID)
	if err != nil {
		h.log.Error("authorize: list personas", "err", err)
		h.provider.WriteAuthorizeError(ctx, w, areq, fosite.ErrServerError)
		return
	}
	if len(personas) == 0 {
		// Onboarding is mandatory before SSO works.
		http.Redirect(w, r, "/onboarding", http.StatusFound)
		return
	}

	clientID := areq.GetClient().GetID()

	// Persona selection: prompt=select_account forces the picker; a saved
	// default (first pick per client) skips it.
	var persona model.Persona
	if !containsPrompt(r.URL.Query().Get("prompt"), "select_account") {
		defaultID, derr := h.defaults.Get(ctx, acctID, clientID)
		if derr == nil {
			for _, p := range personas {
				if p.ID == defaultID {
					persona = p
					break
				}
			}
		} else if derr != oidc.ErrNoDefault {
			h.log.Error("authorize: default persona", "err", derr)
			h.provider.WriteAuthorizeError(ctx, w, areq, fosite.ErrServerError)
			return
		}
	}
	if persona.ID == uuid.Nil {
		// No default (or forced): pick. The picker POSTs the choice to
		// /api/oidc/persona-select and redirects back to this URL.
		next := "/authorize?" + r.URL.RawQuery
		http.Redirect(w, r, "/persona/select?next="+url.QueryEscape(next), http.StatusFound)
		return
	}

	// First-party products: grant every requested scope the client is
	// allowed (fosite's scope strategy rejects the rest). No consent UI.
	for _, scope := range areq.GetRequestedScopes() {
		areq.GrantScope(scope)
	}

	identityVerified := false
	if persona.Identity != nil {
		status, cerr := h.svc.ClaimStatus(ctx, *persona.Identity, acctID)
		if cerr != nil {
			h.log.Error("authorize: claim status", "err", cerr)
			h.provider.WriteAuthorizeError(ctx, w, areq, fosite.ErrServerError)
			return
		}
		identityVerified = status == repository.ClaimVerifiedByAccount
	}

	// The contract: sub = persona ID.
	sess := oidc.NewSession(
		persona.ID.String(),
		acct.ID.String(),
		persona.Name,
		persona.Identity,
		identityVerified,
	)
	sess.Username = persona.Name

	tresp, err := h.provider.NewAuthorizeResponse(ctx, areq, sess)
	if err != nil {
		h.logOAuthErr("authorize response", err)
		h.provider.WriteAuthorizeError(ctx, w, areq, err)
		return
	}
	h.provider.WriteAuthorizeResponse(ctx, w, areq, tresp)
}

// PersonaSelect POST /api/oidc/persona-select — the picker's callback.
// Validates ownership, saves the default, and returns the /authorize URL
// for the browser to follow.
func (h *OIDC) PersonaSelect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acctID, err := h.sessions.Account(ctx, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	var body struct {
		Next      string `json:"next"`
		PersonaID string `json:"persona_id"`
	}
	if err := decode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	// `next` must be a same-origin /authorize URL — never an open redirect.
	nextURL, err := url.Parse(body.Next)
	if err != nil || nextURL.IsAbs() || nextURL.Path != "/authorize" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid next URL"})
		return
	}
	clientID := nextURL.Query().Get("client_id")
	if clientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing client_id"})
		return
	}
	personaID, err := uuid.Parse(body.PersonaID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid persona_id"})
		return
	}
	// Ownership check: the persona must belong to the session account.
	if _, err := h.svc.GetPersona(ctx, personaID, acctID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "persona not found"})
		return
	}
	if err := h.defaults.Set(ctx, acctID, clientID, personaID); err != nil {
		h.log.Error("persona select: save default", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"next": body.Next})
}

// ---- POST /token ----

// Token exchanges codes and refreshes tokens.
func (h *OIDC) Token(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sess := &oidc.Session{}
	areq, err := h.provider.NewAccessRequest(ctx, r, sess)
	if err != nil {
		h.logOAuthErr("access request", err)
		h.provider.WriteAccessError(ctx, w, areq, err)
		return
	}
	aresp, err := h.provider.NewAccessResponse(ctx, areq)
	if err != nil {
		h.logOAuthErr("access response", err)
		h.provider.WriteAccessError(ctx, w, areq, err)
		return
	}
	h.provider.WriteAccessResponse(ctx, w, areq, aresp)
}

// ---- GET /userinfo ----

// UserInfo returns claims for the access token; sub is the persona ID.
func (h *OIDC) UserInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sess := &oidc.Session{}
	// Introspect the bearer token; on success sess holds the persona
	// claims stored at issue time.
	if _, _, err := h.provider.IntrospectToken(ctx, bearerToken(r), fosite.AccessToken, sess); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
		return
	}
	claims := map[string]any{"sub": sess.Subject}
	for k, v := range sess.Extra {
		claims[k] = v
	}
	writeJSON(w, http.StatusOK, claims)
}

// bearerToken extracts the token from the Authorization header.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && h[:len(prefix)] == prefix {
		return h[len(prefix):]
	}
	return r.URL.Query().Get("access_token") // RFC 6750 fallback
}

// ---- POST /revoke ----

// Revoke accepts token revocation (RFC 7009); always 200 per spec.
func (h *OIDC) Revoke(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.provider.NewRevocationRequest(ctx, r); err != nil {
		h.logOAuthErr("revocation", err)
	}
	w.WriteHeader(http.StatusOK)
}

// ---- discovery + JWKS ----

// Discovery serves /.well-known/openid-configuration.
func (h *OIDC) Discovery(w http.ResponseWriter, r *http.Request) {
	iss := h.issuer
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                iss,
		"authorization_endpoint":                iss + "/authorize",
		"token_endpoint":                        iss + "/token",
		"userinfo_endpoint":                     iss + "/userinfo",
		"revocation_endpoint":                   iss + "/revoke",
		"jwks_uri":                              iss + "/jwks.json",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      []string{"openid", "profile", "email"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_basic"},
		"prompt_supported":                      []string{"select_account"},
	})
}

// JWKS serves /.well-known/jwks.json — also linked as /jwks.json.
func (h *OIDC) JWKS(w http.ResponseWriter, r *http.Request) {
	keys, err := h.keys.PublicJWKS(r.Context())
	if err != nil {
		h.log.Error("jwks", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// ---- helpers ----

// logOAuthErr surfaces fosite's Debug/Hint (its Error() hides them).
func (h *OIDC) logOAuthErr(op string, err error) {
	if rfc := fosite.ErrorToRFC6749Error(err); rfc != nil {
		h.log.Warn("oidc", "op", op, "hint", rfc.HintField, "debug", rfc.DebugField)
	} else {
		h.log.Warn("oidc", "op", op, "err", err)
	}
}

// containsPrompt reports whether prompt lists the given value.
func containsPrompt(prompt, value string) bool {
	for _, p := range strings.FieldsFunc(prompt, func(r rune) bool {
		return r == ' ' || r == ','
	}) {
		if p == value {
			return true
		}
	}
	return false
}

package handler

import (
	"log/slog"
	"net/http"

	"account/backend/account/internal/model"
	"account/backend/account/internal/service"
	"account/backend/account/internal/session"
)

// Auth exposes the account lifecycle: register, login, logout, me,
// username/password changes, and the recovery-code flows.
type Auth struct {
	svc      *service.Account
	sessions *session.Store
	log      *slog.Logger
}

// NewAuth wires the handler.
func NewAuth(svc *service.Account, sessions *session.Store, log *slog.Logger) *Auth {
	return &Auth{svc: svc, sessions: sessions, log: log}
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Register POST /api/auth/register
func (h *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := decode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	res, err := h.svc.Register(r.Context(), body.Username, body.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.sessions.Create(w, r.Context(), res.Account.ID); err != nil {
		h.log.Error("session create", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"account":        res.Account,
		"recovery_codes": res.RecoveryCodes,
	})
}

// Login POST /api/auth/login
func (h *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := decode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	acct, err := h.svc.Authenticate(r.Context(), body.Username, body.Password)
	if err != nil {
		// Uniform 401 for both unknown user and wrong password.
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
		return
	}
	if err := h.sessions.Create(w, r.Context(), acct.ID); err != nil {
		h.log.Error("session create", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	personas, err := h.svc.ListPersonas(r.Context(), acct.ID)
	if err != nil {
		h.log.Error("list personas", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"account":  acct,
		"personas": personas,
	})
}

// Logout POST /api/auth/logout
func (h *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.sessions.Destroy(w, r.Context(), r); err != nil {
		h.log.Error("session destroy", "err", err)
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// Me GET /api/me — current account + personas, or 401.
func (h *Auth) Me(w http.ResponseWriter, r *http.Request) {
	acct, err := h.account(w, r)
	if err != nil {
		return
	}
	personas, err := h.svc.ListPersonas(r.Context(), acct.ID)
	if err != nil {
		h.log.Error("list personas", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if personas == nil {
		personas = []model.Persona{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"account":  acct,
		"personas": personas,
	})
}

// ChangeUsername PATCH /api/me
func (h *Auth) ChangeUsername(w http.ResponseWriter, r *http.Request) {
	acct, err := h.account(w, r)
	if err != nil {
		return
	}
	var body struct {
		Username string `json:"username"`
	}
	if err := decode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if err := h.svc.ChangeUsername(r.Context(), acct.ID, body.Username); err != nil {
		writeError(w, err)
		return
	}
	updated, err := h.svc.Get(r.Context(), acct.ID)
	if err != nil {
		h.log.Error("get account", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": updated})
}

// ChangePassword POST /api/me/password
func (h *Auth) ChangePassword(w http.ResponseWriter, r *http.Request) {
	acct, err := h.account(w, r)
	if err != nil {
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if err := h.svc.ChangePassword(r.Context(), acct.ID, body.CurrentPassword, body.NewPassword); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// RegenerateRecoveryCodes POST /api/me/recovery-codes
func (h *Auth) RegenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	acct, err := h.account(w, r)
	if err != nil {
		return
	}
	codes, err := h.svc.RegenerateRecoveryCodes(r.Context(), acct.ID)
	if err != nil {
		h.log.Error("regenerate recovery codes", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"recovery_codes": codes})
}

// ForgotPassword POST /api/auth/forgot-password — always 200.
// The reset itself happens in ResetPassword; this endpoint exists so the
// UI can pace itself and (later) rate-limit per username without leaking
// whether the account exists.
func (h *Auth) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := decode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

// ResetPassword POST /api/auth/reset-password
func (h *Auth) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username     string `json:"username"`
		RecoveryCode string `json:"recovery_code"`
		NewPassword  string `json:"new_password"`
	}
	if err := decode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if err := h.svc.ResetPassword(r.Context(), body.Username, body.RecoveryCode, body.NewPassword); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// account resolves the session account or writes 401.
func (h *Auth) account(w http.ResponseWriter, r *http.Request) (model.Account, error) {
	id, err := h.sessions.Account(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return model.Account{}, err
	}
	acct, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.log.Error("get account", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return model.Account{}, err
	}
	return acct, nil
}

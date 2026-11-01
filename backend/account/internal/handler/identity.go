package handler

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"account/backend/account/internal/mailer"
	"account/backend/account/internal/service"
	"account/backend/account/internal/session"
)

// Identity handles the email-OTP verification of a persona's identity.
type Identity struct {
	svc      *service.Account
	sessions *session.Store
	mailer   *mailer.Mailer
	log      *slog.Logger
}

// NewIdentity wires the handler.
func NewIdentity(svc *service.Account, sessions *session.Store, m *mailer.Mailer, log *slog.Logger) *Identity {
	return &Identity{svc: svc, sessions: sessions, mailer: m, log: log}
}

func (h *Identity) accountID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	acctID, err := h.sessions.Account(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return uuid.Nil, false
	}
	return acctID, true
}

// Request POST /api/personas/{id}/identity/verify — send an OTP email.
func (h *Identity) Request(w http.ResponseWriter, r *http.Request) {
	acctID, ok := h.accountID(w, r)
	if !ok {
		return
	}
	personaID, ok := pathID(r, "id")
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	code, to, err := h.svc.RequestIdentityOTP(r.Context(), acctID, personaID)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.mailer.SendVerificationCode(r.Context(), to, code); err != nil {
		h.log.Error("send otp", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not send verification code"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "code sent"})
}

// Confirm POST /api/personas/{id}/identity/confirm — check the OTP.
func (h *Identity) Confirm(w http.ResponseWriter, r *http.Request) {
	acctID, ok := h.accountID(w, r)
	if !ok {
		return
	}
	personaID, ok := pathID(r, "id")
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := decode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	ev, err := h.svc.ConfirmIdentityOTP(r.Context(), acctID, personaID, body.Code)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "verified",
		"evicted_accounts": ev.AccountsNotified,
		"cleared_personas": ev.PersonasCleared,
	})
}

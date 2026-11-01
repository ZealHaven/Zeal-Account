package handler

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"account/backend/account/internal/model"
	"account/backend/account/internal/service"
	"account/backend/account/internal/session"
)

// Persona exposes persona CRUD for the logged-in account.
type Persona struct {
	svc      *service.Account
	sessions *session.Store
	log      *slog.Logger
}

// NewPersona wires the handler.
func NewPersona(svc *service.Account, sessions *session.Store, log *slog.Logger) *Persona {
	return &Persona{svc: svc, sessions: sessions, log: log}
}

// resolve returns the session's account ID or writes 401.
func (h *Persona) resolve(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	acctID, err := h.sessions.Account(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return uuid.Nil, false
	}
	return acctID, true
}

type personaBody struct {
	Name        *string `json:"name"`
	Identity    *string `json:"identity"`
	Description *string `json:"description"`
}

// List GET /api/personas
func (h *Persona) List(w http.ResponseWriter, r *http.Request) {
	acctID, ok := h.resolve(w, r)
	if !ok {
		return
	}
	personas, err := h.svc.ListPersonas(r.Context(), acctID)
	if err != nil {
		h.log.Error("list personas", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if personas == nil {
		personas = []model.Persona{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"personas": personas})
}

// Create POST /api/personas
func (h *Persona) Create(w http.ResponseWriter, r *http.Request) {
	acctID, ok := h.resolve(w, r)
	if !ok {
		return
	}
	var body personaBody
	if err := decode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if body.Name == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": service.ErrInvalidName.Error()})
		return
	}
	p, err := h.svc.CreatePersona(r.Context(), acctID, *body.Name, body.Identity, body.Description)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// Update PATCH /api/personas/{id}
func (h *Persona) Update(w http.ResponseWriter, r *http.Request) {
	acctID, ok := h.resolve(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	var body personaBody
	if err := decode(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	p, err := h.svc.UpdatePersona(r.Context(), id, acctID, body.Name, body.Identity, body.Description)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

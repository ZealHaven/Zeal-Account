package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"account/backend/account/internal/repository"
	"account/backend/account/internal/service"
)

// writeJSON writes a JSON response body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError maps domain errors to HTTP status codes and a stable message.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidUsername),
		errors.Is(err, service.ErrInvalidPassword),
		errors.Is(err, service.ErrInvalidName),
		errors.Is(err, service.ErrWrongPassword),
		errors.Is(err, service.ErrBadRecoveryCode),
		errors.Is(err, service.ErrIdentityTaken):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, repository.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "username is already taken"})
	case errors.Is(err, repository.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	default:
		// Never leak internals to the client; the server logs separately.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

// decode reads a JSON body, rejecting unknown fields and oversized payloads.
func decode(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// pathID reads a UUID path parameter; ok=false on malformed values.
func pathID(r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	return id, err == nil
}

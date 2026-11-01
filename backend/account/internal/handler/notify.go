package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"account/backend/account/internal/repository"
	"account/backend/account/internal/session"
)

// Notify exposes the in-app notification inbox.
type Notify struct {
	repo     *repository.Notification
	sessions *session.Store
	log      *slog.Logger
}

// NewNotify wires the handler.
func NewNotify(repo *repository.Notification, sessions *session.Store, log *slog.Logger) *Notify {
	return &Notify{repo: repo, sessions: sessions, log: log}
}

func (h *Notify) accountID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	acctID, err := h.sessions.Account(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return uuid.Nil, false
	}
	return acctID, true
}

// List GET /api/notifications?unread=true&limit=50
func (h *Notify) List(w http.ResponseWriter, r *http.Request) {
	acctID, ok := h.accountID(w, r)
	if !ok {
		return
	}
	unreadOnly := r.URL.Query().Get("unread") == "true"
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.repo.List(r.Context(), acctID, unreadOnly, limit)
	if err != nil {
		h.log.Error("notification list", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": items})
}

// UnreadCount GET /api/notifications/unread-count
func (h *Notify) UnreadCount(w http.ResponseWriter, r *http.Request) {
	acctID, ok := h.accountID(w, r)
	if !ok {
		return
	}
	n, err := h.repo.UnreadCount(r.Context(), acctID)
	if err != nil {
		h.log.Error("notification unread count", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"unread": n})
}

// MarkRead POST /api/notifications/{id}/read
func (h *Notify) MarkRead(w http.ResponseWriter, r *http.Request) {
	acctID, ok := h.accountID(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err := h.repo.MarkRead(r.Context(), id, acctID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// MarkAllRead POST /api/notifications/read-all
func (h *Notify) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	acctID, ok := h.accountID(w, r)
	if !ok {
		return
	}
	if err := h.repo.MarkAllRead(r.Context(), acctID); err != nil {
		h.log.Error("notification mark all read", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

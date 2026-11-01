// Package session stores login sessions in Redis, keyed by an opaque
// httpOnly cookie value. The stored value is the account ID; the cookie
// itself is random and unguessable (sessions are looked up server-side).
package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	// CookieName is the session cookie.
	CookieName = "account_session"
	// TTL is how long a session lives without activity.
	TTL = 30 * 24 * time.Hour
)

// ErrMissing is returned when no valid session exists.
var ErrMissing = errors.New("session: missing or expired")

// Store reads and writes login sessions.
type Store struct {
	rdb    *redis.Client
	secure bool // Secure flag for the cookie
}

// New creates a Store. secure should be true behind HTTPS.
func New(rdb *redis.Client, secure bool) *Store {
	return &Store{rdb: rdb, secure: secure}
}

// Create makes a new session for the account and sets the cookie.
func (s *Store) Create(w http.ResponseWriter, ctx context.Context, accountID uuid.UUID) error {
	raw, err := randomToken()
	if err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, key(raw), accountID.String(), TTL).Err(); err != nil {
		return fmt.Errorf("session create: %w", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(TTL / time.Second),
	})
	return nil
}

// Account resolves the session from a request cookie.
// Returns ErrMissing when absent or expired.
func (s *Store) Account(ctx context.Context, r *http.Request) (uuid.UUID, error) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return uuid.Nil, ErrMissing
	}
	val, err := s.rdb.Get(ctx, key(c.Value)).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, ErrMissing
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("session get: %w", err)
	}
	id, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, fmt.Errorf("session parse: %w", err)
	}
	return id, nil
}

// Destroy removes the session for the cookie on the request and clears it.
func (s *Store) Destroy(w http.ResponseWriter, ctx context.Context, r *http.Request) error {
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		if err := s.rdb.Del(ctx, key(c.Value)).Err(); err != nil {
			return fmt.Errorf("session destroy: %w", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	return nil
}

func key(raw string) string { return "session:" + raw }

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

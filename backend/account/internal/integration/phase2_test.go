//go:build integration

// Phase 2: identity claim-then-verify + notifications.
// Run with: go test -tags=integration -run TestPhase2 ./...
package integration_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"account/backend/account/internal/db"
	"account/backend/account/internal/mailer"
	"account/backend/account/internal/repository"
	"account/backend/account/internal/server"
)

// startServer boots an API-only server against the real infra, truncated.
func startServer(t *testing.T) (*httptest.Server, *repository.Notification, func()) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("DATABASE_URL/REDIS_URL not set")
	}
	ctx := context.Background()
	pool, err := db.NewPostgres(ctx, dbURL)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	rdb, err := db.NewRedis(ctx, redisURL)
	if err != nil {
		pool.Close()
		t.Fatalf("redis: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE notifications, account_identities, recovery_codes, personas, accounts CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	log := slog.New(slog.DiscardHandler)
	// SMTP is unset → the mailer logs codes instead of sending; the
	// repository's HashOTP path is exercised directly below.
	h, err := server.New(server.Deps{
		FrontendURL: "",
		Pool:        pool,
		RDB:         rdb,
		Secure:      false,
		SMTP:        mailer.Config{},
		Log:         log,
	})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	srv := httptest.NewServer(h)
	notifyRepo := repository.NewNotification(pool)
	return srv, notifyRepo, func() {
		srv.Close()
		pool.Close()
		rdb.Close()
	}
}

// accountIDOf reads the session account's ID via /api/me.
func accountIDOf(t *testing.T, c *client) uuid.UUID {
	t.Helper()
	resp, body := c.do("GET", "/api/me", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("me: %d %v", resp.StatusCode, body)
	}
	id, err := uuid.Parse(body["account"].(map[string]any)["id"].(string))
	if err != nil {
		t.Fatalf("account id: %v", err)
	}
	return id
}

func TestPhase2IdentityEviction(t *testing.T) {
	srv, _, cleanup := startServer(t)
	defer cleanup()

	ctx := context.Background()
	ts := time.Now().UnixNano() % 1_000_000_000
	first := fmt.Sprintf("alice%d", ts)
	second := fmt.Sprintf("bob%d", ts)
	identity := fmt.Sprintf("shared-%d@example.com", ts)

	ca := newClient(t, srv.URL)
	cb := newClient(t, srv.URL)

	// Register both; each creates a persona claiming the same identity.
	if resp, body := ca.do("POST", "/api/auth/register", map[string]string{"username": first, "password": "password one two"}); resp.StatusCode != 201 {
		t.Fatalf("register A: %d %v", resp.StatusCode, body)
	}
	if resp, body := cb.do("POST", "/api/auth/register", map[string]string{"username": second, "password": "password one two"}); resp.StatusCode != 201 {
		t.Fatalf("register B: %d %v", resp.StatusCode, body)
	}
	resp, body := ca.do("POST", "/api/personas", map[string]any{"name": "A-persona", "identity": identity})
	if resp.StatusCode != 201 {
		t.Fatalf("persona A: %d %v", resp.StatusCode, body)
	}
	personaA, _ := body["id"].(string)
	resp, body = cb.do("POST", "/api/personas", map[string]any{"name": "B-persona", "identity": identity})
	if resp.StatusCode != 201 {
		t.Fatalf("persona B: %d %v", resp.StatusCode, body)
	}
	personaB, _ := body["id"].(string)

	// Request OTP for A — SMTP unset, so the code is only logged. Drive
	// verification through the repository to inject the real code.
	resp, body = ca.do("POST", "/api/personas/"+personaA+"/identity/verify", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("verify request A: %d %v", resp.StatusCode, body)
	}

	// Build a matching OTP through the same hash path and set it on A's
	// claim, then confirm via the API to exercise the full transaction.
	pool, err := db.NewPostgres(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	claims := repository.NewIdentityClaim(pool)
	plain, hash, err := repository.HashOTP()
	if err != nil {
		t.Fatalf("hash otp: %v", err)
	}
	if err := claims.SetOTP(ctx, identity, accountIDOf(t, ca), hash); err != nil {
		t.Fatalf("set otp: %v", err)
	}

	// Wrong code → 400, nothing evicted.
	resp, body = ca.do("POST", "/api/personas/"+personaA+"/identity/confirm", map[string]string{"code": "000000"})
	if resp.StatusCode != 400 {
		t.Fatalf("confirm wrong code: want 400, got %d %v", resp.StatusCode, body)
	}

	// Correct code → verified; B loses the claim + persona identity AND
	// gets a notification.
	resp, body = ca.do("POST", "/api/personas/"+personaA+"/identity/confirm", map[string]string{"code": plain})
	if resp.StatusCode != 200 {
		t.Fatalf("confirm correct code: %d %v", resp.StatusCode, body)
	}
	if got := body["evicted_accounts"]; fmt.Sprint(got) != "1" {
		t.Fatalf("evicted_accounts = %v, want 1", got)
	}

	// B's persona lost its identity.
	resp, body = cb.do("GET", "/api/personas", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("B personas: %d", resp.StatusCode)
	}
	personas := body["personas"].([]any)
	if len(personas) != 1 {
		t.Fatalf("B personas: want 1, got %d", len(personas))
	}
	if id := personas[0].(map[string]any)["identity"]; id != nil {
		t.Fatalf("B persona identity should be cleared, got %v", id)
	}

	// B has an unread identity_claim_removed notification naming the persona.
	resp, body = cb.do("GET", "/api/notifications?unread=true", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("B notifications: %d", resp.StatusCode)
	}
	notifs := body["notifications"].([]any)
	if len(notifs) != 1 {
		t.Fatalf("B notifications: want 1, got %d", len(notifs))
	}
	n := notifs[0].(map[string]any)
	if n["type"] != "identity_claim_removed" {
		t.Fatalf("notification type = %v", n["type"])
	}
	data := n["data"].(map[string]any)
	if data["identity"] != identity {
		t.Fatalf("notification identity = %v", data["identity"])
	}
	// persona_ids must contain B's persona — the notification is actionable.
	ids := data["persona_ids"].([]any)
	found := false
	for _, v := range ids {
		if v == personaB {
			found = true
		}
	}
	if !found {
		t.Fatalf("persona_ids %v missing %s", ids, personaB)
	}

	// Unread count is 1 for B…
	resp, body = cb.do("GET", "/api/notifications/unread-count", nil)
	if resp.StatusCode != 200 || body["unread"].(float64) != 1 {
		t.Fatalf("B unread count: %d %v", resp.StatusCode, body)
	}
	// …and 0 for A (the winner is not notified).
	resp, body = ca.do("GET", "/api/notifications/unread-count", nil)
	if resp.StatusCode != 200 || body["unread"].(float64) != 0 {
		t.Fatalf("A unread count: %d %v", resp.StatusCode, body)
	}

	// Mark read → badge clears; idempotent second mark is fine.
	notID := n["id"].(string)
	resp, _ = cb.do("POST", "/api/notifications/"+notID+"/read", nil)
	if resp.StatusCode != 204 {
		t.Fatalf("mark read: %d", resp.StatusCode)
	}
	resp, _ = cb.do("POST", "/api/notifications/"+notID+"/read", nil)
	if resp.StatusCode != 204 {
		t.Fatalf("mark read again: %d", resp.StatusCode)
	}
	resp, body = cb.do("GET", "/api/notifications/unread-count", nil)
	if body["unread"].(float64) != 0 {
		t.Fatalf("B unread after read = %v", body["unread"])
	}

	// Once an identity is VERIFIED by an account, no other account can
	// even set it on a persona (the requirement: "other account can't use
	// it") — the claim is refused outright.
	resp, body = cb.do("POST", "/api/personas", map[string]any{"name": "reclaim", "identity": identity})
	if resp.StatusCode != 400 {
		t.Fatalf("re-claim verified identity: want 400, got %d %v", resp.StatusCode, body)
	}

	// A re-verifying its own already-verified identity is a no-op 400.
	resp, body = ca.do("POST", "/api/personas/"+personaA+"/identity/verify", nil)
	if resp.StatusCode != 400 {
		t.Fatalf("A re-verify: want 400 (already verified), got %d %v", resp.StatusCode, body)
	}
}

// TestPhase2SecurityNotifications ensures password changes/resets fire
// in-app notifications.
func TestPhase2SecurityNotifications(t *testing.T) {
	srv, _, cleanup := startServer(t)
	defer cleanup()

	ts := time.Now().UnixNano() % 1_000_000_000
	user := fmt.Sprintf("sec%d", ts)
	c := newClient(t, srv.URL)

	resp, body := c.do("POST", "/api/auth/register", map[string]string{"username": user, "password": "password one two"})
	if resp.StatusCode != 201 {
		t.Fatalf("register: %d %v", resp.StatusCode, body)
	}

	// Change password → notification.
	resp, body = c.do("POST", "/api/me/password", map[string]string{
		"current_password": "password one two",
		"new_password":     "password three four",
	})
	if resp.StatusCode != 204 {
		t.Fatalf("change password: %d %v", resp.StatusCode, body)
	}

	// Regenerate codes → notification. The old codes are invalidated, so
	// the reset below must use a code from the NEW set.
	resp, body = c.do("POST", "/api/me/recovery-codes", nil)
	if resp.StatusCode != 201 {
		t.Fatalf("regen codes: %d", resp.StatusCode)
	}
	newCodes := body["recovery_codes"].([]any)

	resp, body = c.do("GET", "/api/notifications", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("notifications: %d", resp.StatusCode)
	}
	notifs := body["notifications"].([]any)
	if len(notifs) != 2 {
		t.Fatalf("want 2 security notifications, got %d", len(notifs))
	}

	// Reset via a fresh recovery code → notification.
	other := newClient(t, srv.URL)
	resp, body = other.do("POST", "/api/auth/reset-password", map[string]any{
		"username":      user,
		"recovery_code": newCodes[0],
		"new_password":  "password five six",
	})
	if resp.StatusCode != 204 {
		t.Fatalf("reset: %d %v", resp.StatusCode, body)
	}
	resp, body = c.do("GET", "/api/notifications", nil)
	if got := len(body["notifications"].([]any)); got != 3 {
		t.Fatalf("want 3 notifications after reset, got %d", got)
	}
}

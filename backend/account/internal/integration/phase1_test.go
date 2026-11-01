//go:build integration

// End-to-end API tests against real Postgres + Redis.
// Run with: go test -tags=integration ./...
// Requires: docker compose up, migrations applied (just up && just migrate-up).
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"log/slog"

	"account/backend/account/internal/db"
	"account/backend/account/internal/server"
)

// client keeps a cookie jar so sessions persist across calls.
type client struct {
	t    *testing.T
	base string
	hc   *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &client{
		t:    t,
		base: base,
		hc:   &http.Client{Jar: jar, Timeout: 10 * time.Second},
	}
}

func (c *client) do(method, path string, body any) (*http.Response, map[string]any) {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestAccountPhase1(t *testing.T) {
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
	defer pool.Close()
	// Isolate this run from any dev data.
	if _, err := pool.Exec(ctx, `TRUNCATE recovery_codes, personas, accounts CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	rdb, err := db.NewRedis(ctx, redisURL)
	if err != nil {
		t.Fatalf("redis: %v", err)
	}
	defer rdb.Close()
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	log := slog.New(slog.DiscardHandler)
	handler, err := server.New(server.Deps{
		FrontendURL: "", // API only — frontend isn't under test
		Pool:        pool,
		RDB:         rdb,
		Secure:      false,
		Log:         log,
	})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()

	user := fmt.Sprintf("u%d", time.Now().UnixNano()%1_000_000_000)
	c := newClient(t, srv.URL)

	// --- register ------------------------------------------------------------
	resp, body := c.do("POST", "/api/auth/register", map[string]string{
		"username": user,
		"password": "correct horse battery",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("register: %d %v", resp.StatusCode, body)
	}
	codesRaw, ok := body["recovery_codes"].([]any)
	if !ok || len(codesRaw) != 10 {
		t.Fatalf("register: want 10 recovery codes, got %v", body["recovery_codes"])
	}
	var codes []string
	for _, v := range codesRaw {
		codes = append(codes, v.(string))
	}

	// Session cookie must make /api/me work immediately.
	resp, body = c.do("GET", "/api/me", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("me after register: %d %v", resp.StatusCode, body)
	}
	if got := body["account"].(map[string]any)["username"].(string); got != user {
		t.Fatalf("me: username = %q, want %q", got, user)
	}
	if len(body["personas"].([]any)) != 0 {
		t.Fatalf("me: want 0 personas before onboarding, got %v", body["personas"])
	}

	// --- duplicate username rejected ----------------------------------------
	resp, body = c.do("POST", "/api/auth/register", map[string]string{
		"username": strings.ToUpper(user), // case-insensitive collision
		"password": "correct horse battery",
	})
	if resp.StatusCode != 409 {
		t.Fatalf("duplicate register: want 409, got %d %v", resp.StatusCode, body)
	}

	// --- weak password rejected ---------------------------------------------
	resp, body = c.do("POST", "/api/auth/register", map[string]string{
		"username": user + "2",
		"password": "short",
	})
	if resp.StatusCode != 400 {
		t.Fatalf("weak password: want 400, got %d %v", resp.StatusCode, body)
	}

	// --- persona CRUD --------------------------------------------------------
	resp, body = c.do("POST", "/api/personas", map[string]any{
		"name":        "Main",
		"identity":    "person@example.com",
		"description": "Primary persona",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("persona create: %d %v", resp.StatusCode, body)
	}
	personaID, _ := body["id"].(string)
	if personaID == "" {
		t.Fatalf("persona create: no id in %v", body)
	}

	// Empty identity becomes null.
	resp, body = c.do("POST", "/api/personas", map[string]any{
		"name":     "Anon",
		"identity": "  ",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("persona create (empty identity): %d %v", resp.StatusCode, body)
	}
	if body["identity"] != nil {
		t.Fatalf("empty identity should be null, got %v", body["identity"])
	}

	resp, body = c.do("GET", "/api/personas", nil)
	if resp.StatusCode != 200 || len(body["personas"].([]any)) != 2 {
		t.Fatalf("persona list: %d %v", resp.StatusCode, body)
	}

	// A pending claim by another account is ALLOWED (claim-then-verify:
	// exclusivity only kicks in after email OTP verification). Phase 2's
	// test covers the eviction; here we only assert no false rejection.
	other := newClient(t, srv.URL)
	resp, _ = other.do("POST", "/api/auth/register", map[string]string{
		"username": user + "-other",
		"password": "correct horse battery",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("other register: %d", resp.StatusCode)
	}
	resp, body = other.do("POST", "/api/personas", map[string]any{
		"name":     "Squatter",
		"identity": "person@example.com",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("pending identity claim: want 201, got %d %v", resp.StatusCode, body)
	}

	// PATCH: rename + clear identity (empty string clears).
	resp, body = c.do("PATCH", "/api/personas/"+personaID, map[string]any{
		"name":     "Renamed",
		"identity": "",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("persona update: %d %v", resp.StatusCode, body)
	}
	if body["name"] != "Renamed" || body["identity"] != nil {
		t.Fatalf("persona update: got name=%v identity=%v", body["name"], body["identity"])
	}

	// Another account's persona is invisible/not patchable.
	resp, _ = other.do("PATCH", "/api/personas/"+personaID, map[string]any{"name": "Hacked"})
	if resp.StatusCode != 404 {
		t.Fatalf("cross-account patch: want 404, got %d", resp.StatusCode)
	}

	// --- username change -----------------------------------------------------
	resp, body = c.do("PATCH", "/api/me", map[string]string{"username": user + "-renamed"})
	if resp.StatusCode != 200 {
		t.Fatalf("username change: %d %v", resp.StatusCode, body)
	}

	// Session survives rename; new username needed for login.
	resp, _ = c.do("POST", "/api/auth/login", map[string]string{
		"username": user + "-renamed",
		"password": "correct horse battery",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("login after rename: %d", resp.StatusCode)
	}

	// --- password change -----------------------------------------------------
	resp, body = c.do("POST", "/api/me/password", map[string]string{
		"current_password": "correct horse battery",
		"new_password":     "even more correct horse",
	})
	if resp.StatusCode != 204 {
		t.Fatalf("password change: %d %v", resp.StatusCode, body)
	}

	// Old password no longer works.
	fresh := newClient(t, srv.URL)
	resp, _ = fresh.do("POST", "/api/auth/login", map[string]string{
		"username": user + "-renamed",
		"password": "correct horse battery",
	})
	if resp.StatusCode != 401 {
		t.Fatalf("login with old password: want 401, got %d", resp.StatusCode)
	}

	// Wrong current password refused.
	resp, _ = c.do("POST", "/api/me/password", map[string]string{
		"current_password": "not the password",
		"new_password":     "another password",
	})
	if resp.StatusCode != 400 {
		t.Fatalf("wrong current password: want 400, got %d", resp.StatusCode)
	}

	// --- forgot password via recovery code -----------------------------------
	// Logged-out client: old password now invalid; recover with a code.
	resp, body = fresh.do("POST", "/api/auth/forgot-password", map[string]string{
		"username": user + "-renamed",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("forgot-password: %d %v", resp.StatusCode, body)
	}

	resp, body = fresh.do("POST", "/api/auth/reset-password", map[string]string{
		"username":      user + "-renamed",
		"recovery_code": "garbage-code-value",
		"new_password":  "a brand new password",
	})
	if resp.StatusCode != 400 {
		t.Fatalf("reset with bad code: want 400, got %d %v", resp.StatusCode, body)
	}

	resp, body = fresh.do("POST", "/api/auth/reset-password", map[string]string{
		"username":      user + "-renamed",
		"recovery_code": codes[0],
		"new_password":  "a brand new password",
	})
	if resp.StatusCode != 204 {
		t.Fatalf("reset with real code: %d %v", resp.StatusCode, body)
	}

	// Same code cannot be reused.
	resp, _ = fresh.do("POST", "/api/auth/reset-password", map[string]string{
		"username":      user + "-renamed",
		"recovery_code": codes[0],
		"new_password":  "yet another password",
	})
	if resp.StatusCode != 400 {
		t.Fatalf("reused recovery code: want 400, got %d", resp.StatusCode)
	}

	// New password logs in.
	resp, _ = fresh.do("POST", "/api/auth/login", map[string]string{
		"username": user + "-renamed",
		"password": "a brand new password",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("login after reset: %d", resp.StatusCode)
	}

	// --- regenerate invalidates old codes ------------------------------------
	resp, body = c.do("POST", "/api/me/recovery-codes", nil)
	if resp.StatusCode != 201 {
		t.Fatalf("regenerate: %d %v", resp.StatusCode, body)
	}
	resp, _ = fresh.do("POST", "/api/auth/reset-password", map[string]string{
		"username":      user + "-renamed",
		"recovery_code": codes[1], // untouched old code
		"new_password":  "third password",
	})
	if resp.StatusCode != 400 {
		t.Fatalf("stale code after regen: want 400, got %d", resp.StatusCode)
	}

	// --- logout --------------------------------------------------------------
	resp, _ = c.do("POST", "/api/auth/logout", nil)
	if resp.StatusCode != 204 {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	resp, _ = c.do("GET", "/api/me", nil)
	if resp.StatusCode != 401 {
		t.Fatalf("me after logout: want 401, got %d", resp.StatusCode)
	}
}

//go:build integration

// Phase 3: the OIDC provider. Full authorization-code + PKCE loop against
// the real server, asserting the core contract: `sub` == persona ID.
// Run with: go test -tags=integration -run TestPhase3 ./...
package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"account/backend/account/internal/db"
	"account/backend/account/internal/mailer"
	"account/backend/account/internal/server"
)

// startOIDCServer boots a server whose issuer matches its own URL.
func startOIDCServer(t *testing.T) (*httptest.Server, func()) {
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
	if _, err := pool.Exec(ctx, `
		TRUNCATE notifications, account_identities, persona_defaults,
		         fosite_requests, oauth_access_tokens, oauth_client_assertions,
		         recovery_codes, personas, accounts, signing_keys CASCADE
	`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// NewUnstartedServer reserves the listener immediately (URL is only
	// set on Start, but the address is known), so the issuer can be baked
	// into the handler before Start.
	srv := httptest.NewUnstartedServer(nil)
	issuer := "http://" + srv.Listener.Addr().String()
	log := slog.New(slog.DiscardHandler)
	h, err := server.New(server.Deps{
		FrontendURL:  "",
		Pool:         pool,
		RDB:          rdb,
		Secure:       false,
		Issuer:       issuer,
		KeyEncHex:    testKeyEncHex,
		GlobalSecret: testGlobalSecret,
		SMTP:         mailer.Config{},
		Log:          log,
	})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	srv.Config.Handler = h
	srv.Start()

	return srv, func() {
		srv.Close()
		pool.Close()
		rdb.Close()
	}
}

// pkcePair returns (verifier, S256 challenge).
func pkcePair(t *testing.T) (string, string) {
	t.Helper()
	verifier, err := randomURLSafe(64)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// decodeJWTClaims parses a JWT payload without verifying the signature
// (verification is asserted separately via JWKS presence + issuer match).
func decodeJWTClaims(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed JWT: %d parts", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("jwt payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("jwt unmarshal: %v", err)
	}
	return claims
}

// followAuth hits /authorize WITHOUT following redirects, returning the
// 3xx Location (or the final status if not a redirect).
func followAuth(t *testing.T, hc *http.Client, target string) (int, string) {
	t.Helper()
	noRedirect := *hc
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := noRedirect.Get(target)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Location")
}

func TestPhase3OIDCFlow(t *testing.T) {
	srv, cleanup := startOIDCServer(t)
	defer cleanup()
	issuer := srv.URL

	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar, Timeout: 15 * time.Second}

	// --- account + persona ---------------------------------------------------
	ts := time.Now().UnixNano() % 1_000_000_000
	user := fmt.Sprintf("oidc%d", ts)
	resp, body := postJSON(t, hc, issuer+"/api/auth/register", map[string]string{
		"username": user, "password": "password one two",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("register: %d %v", resp.StatusCode, body)
	}
	accountID := body["account"].(map[string]any)["id"].(string)

	resp, body = postJSON(t, hc, issuer+"/api/personas", map[string]any{
		"name": "OIDC Persona", "identity": fmt.Sprintf("oidc-%d@example.com", ts),
	})
	if resp.StatusCode != 201 {
		t.Fatalf("persona: %d %v", resp.StatusCode, body)
	}
	personaID := body["id"].(string)

	// --- discovery + jwks ----------------------------------------------------
	resp, body = getJSON(t, hc, issuer+"/.well-known/openid-configuration")
	if resp.StatusCode != 200 {
		t.Fatalf("discovery: %d", resp.StatusCode)
	}
	if body["issuer"] != issuer {
		t.Fatalf("discovery issuer = %v, want %v", body["issuer"], issuer)
	}
	resp, jwks := getJSON(t, hc, issuer+"/.well-known/jwks.json")
	if resp.StatusCode != 200 || len(jwks["keys"].([]any)) == 0 {
		t.Fatalf("jwks: %d %v", resp.StatusCode, jwks)
	}

	// --- authorize: first hit → picker (no default persona yet) -------------
	verifier, challenge := pkcePair(t)
	authQuery := url.Values{
		"response_type":         {"code"},
		"client_id":             {"zeal-test"},
		"redirect_uri":          {"http://127.0.0.1:9999/callback"},
		"scope":                 {"openid profile"},
		"state":                 {"state-123456"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	status, loc := followAuth(t, hc, issuer+"/authorize?"+authQuery)
	if !isRedirect(status) || !strings.HasPrefix(loc, "/persona/select?") {
		t.Fatalf("first authorize: want picker redirect, got %d %q", status, loc)
	}

	// --- picker: POST the choice, then retry authorize ----------------------
	pickerNext := mustQueryParam(t, loc, "next")
	resp, body = postJSON(t, hc, issuer+"/api/oidc/persona-select", map[string]string{
		"next": pickerNext, "persona_id": personaID,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("persona select: %d %v", resp.StatusCode, body)
	}

	status, loc = followAuth(t, hc, issuer+"/authorize?"+authQuery)
	if !isRedirect(status) || !strings.HasPrefix(loc, "http://127.0.0.1:9999/callback") {
		t.Fatalf("second authorize: want code redirect, got %d %q", status, loc)
	}
	code := mustQueryParam(t, loc, "code")
	if code == "" {
		t.Fatalf("no code in %q", loc)
	}

	// --- token exchange ------------------------------------------------------
	resp, body = postForm(t, hc, issuer+"/token", map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  "http://127.0.0.1:9999/callback",
		"client_id":     "zeal-test",
		"code_verifier": verifier,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("token: %d %v", resp.StatusCode, body)
	}
	accessToken, _ := body["access_token"].(string)
	idToken, _ := body["id_token"].(string)
	refreshToken, _ := body["refresh_token"].(string)
	if accessToken == "" || idToken == "" || refreshToken == "" {
		t.Fatalf("token response missing fields: %v", body)
	}

	// --- THE CONTRACT: sub == persona ID ------------------------------------
	claims := decodeJWTClaims(t, idToken)
	if claims["sub"] != personaID {
		t.Fatalf("id_token sub = %v, want persona ID %v", claims["sub"], personaID)
	}
	if claims["iss"] != issuer {
		t.Fatalf("id_token iss = %v, want %v", claims["iss"], issuer)
	}
	if claims["zeal_account_id"] != accountID {
		t.Fatalf("zeal_account_id = %v, want %v", claims["zeal_account_id"], accountID)
	}
	if claims["zeal_persona_name"] != "OIDC Persona" {
		t.Fatalf("zeal_persona_name = %v", claims["zeal_persona_name"])
	}
	// Identity claim present + verification flag reported.
	if claims["zeal_persona_identity"] != fmt.Sprintf("oidc-%d@example.com", ts) {
		t.Fatalf("zeal_persona_identity = %v", claims["zeal_persona_identity"])
	}
	if _, ok := claims["zeal_persona_identity_verified"]; !ok {
		t.Fatalf("missing zeal_persona_identity_verified claim")
	}

	// --- userinfo ------------------------------------------------------------
	req, _ := http.NewRequest("GET", issuer+"/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp2, err := hc.Do(req)
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	defer resp2.Body.Close()
	var uinfo map[string]any
	_ = json.NewDecoder(resp2.Body).Decode(&uinfo)
	if resp2.StatusCode != 200 || uinfo["sub"] != personaID {
		t.Fatalf("userinfo: %d %v", resp2.StatusCode, uinfo)
	}

	// --- refresh grant -------------------------------------------------------
	// Note: replaying the code (below) revokes the whole token family —
	// that's fosite's code-replay defense — so refresh is tested FIRST.
	resp, body = postForm(t, hc, issuer+"/token", map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     "zeal-test",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("refresh: %d %v", resp.StatusCode, body)
	}
	claims = decodeJWTClaims(t, body["id_token"].(string))
	if claims["sub"] != personaID {
		t.Fatalf("refreshed id_token sub = %v", claims["sub"])
	}

	// --- replay: the code is single-use (revokes the token family) ----------
	resp, body = postForm(t, hc, issuer+"/token", map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  "http://127.0.0.1:9999/callback",
		"client_id":     "zeal-test",
		"code_verifier": verifier,
	})
	if resp.StatusCode != 400 {
		t.Fatalf("code replay: want 400, got %d %v", resp.StatusCode, body)
	}

	// --- PKCE: wrong verifier rejected ---------------------------------------
	status, loc = followAuth(t, hc, issuer+"/authorize?"+authQuery)
	if !isRedirect(status) || !strings.HasPrefix(loc, "http://127.0.0.1:9999/callback") {
		t.Fatalf("authorize (default persona now saved): %d %q", status, loc)
	}
	code2 := mustQueryParam(t, loc, "code")
	resp, body = postForm(t, hc, issuer+"/token", map[string]string{
		"grant_type":    "authorization_code",
		"code":          code2,
		"redirect_uri":  "http://127.0.0.1:9999/callback",
		"client_id":     "zeal-test",
		"code_verifier": "totally-wrong-verifier-that-is-long-enough-to-pass-length-check",
	})
	if resp.StatusCode != 400 {
		t.Fatalf("wrong PKCE verifier: want 400, got %d %v", resp.StatusCode, body)
	}

	// --- prompt=select_account forces the picker again ------------------------
	forced := authQuery + "&prompt=select_account"
	status, loc = followAuth(t, hc, issuer+"/authorize?"+forced)
	if !isRedirect(status) || !strings.HasPrefix(loc, "/persona/select?") {
		t.Fatalf("prompt=select_account: want picker, got %d %q", status, loc)
	}

	// --- anonymous authorize → redirected to login ----------------------------
	anon := &http.Client{Timeout: 15 * time.Second}
	status, loc = followAuth(t, anon, issuer+"/authorize?"+authQuery)
	if !isRedirect(status) || !strings.HasPrefix(loc, "/login?next=") {
		t.Fatalf("anonymous authorize: want login redirect, got %d %q", status, loc)
	}

	// --- unknown client → refused WITHOUT a redirect (redirect_uri can't
	// be trusted for an unknown client, so fosite answers directly) ---------
	bad := strings.Replace(authQuery, "client_id=zeal-test", "client_id=nope", 1)
	status, loc = followAuth(t, hc, issuer+"/authorize?"+bad)
	if isRedirect(status) {
		t.Fatalf("unknown client: must not redirect, got %d %q", status, loc)
	}
	if status != 401 && status != 400 {
		t.Fatalf("unknown client: want 400/401, got %d", status)
	}
}

// isRedirect reports 302/303 — both are valid for authorize redirects.
func isRedirect(status int) bool { return status == 302 || status == 303 }

// postJSON sends a JSON POST with the client's cookie jar.
func postJSON(t *testing.T, hc *http.Client, endpoint string, payload any) (*http.Response, map[string]any) {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := hc.Post(endpoint, "application/json", strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// postForm sends an application/x-www-form-urlencoded POST (the /token
// endpoint parses the body as a form, not JSON).
func postForm(t *testing.T, hc *http.Client, endpoint string, form map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	vals := url.Values{}
	for k, v := range form {
		vals.Set(k, v)
	}
	resp, err := hc.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(vals.Encode()))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// getJSON sends a GET and decodes the body.
func getJSON(t *testing.T, hc *http.Client, endpoint string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := hc.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// mustQueryParam extracts a query parameter from a URL string.
func mustQueryParam(t *testing.T, rawURL, key string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}
	return u.Query().Get(key)
}

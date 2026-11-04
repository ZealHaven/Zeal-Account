package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The frontend proxy rewrites Host to the loopback target, so it must
// hand the app behind it the browser-facing origin via X-Forwarded-*:
// adapter-node derives its self-origin from those headers (CSRF checks,
// absolute URLs) and would otherwise see http://127.0.0.1:<port>.
func TestFrontendProxySetsForwardedOrigin(t *testing.T) {
	var gotHost, gotFwdHost, gotProto string
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotFwdHost = r.Header.Get("X-Forwarded-Host")
		gotProto = r.Header.Get("X-Forwarded-Proto")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer frontend.Close()

	targetHost := strings.TrimPrefix(frontend.URL, "http://")

	for _, tc := range []struct {
		name   string
		secure bool
		proto  string
	}{
		{name: "prod (TLS terminated upstream)", secure: true, proto: "https"},
		{name: "dev (plain HTTP)", secure: false, proto: "http"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxy, err := newFrontendProxy(frontend.URL, tc.secure)
			if err != nil {
				t.Fatalf("newFrontendProxy: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "http://account.example/portal", nil)
			req.Host = "account.example"
			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
			}
			if gotHost != targetHost {
				t.Errorf("outgoing Host = %q, want %q (loopback target)", gotHost, targetHost)
			}
			if gotFwdHost != "account.example" {
				t.Errorf("X-Forwarded-Host = %q, want %q", gotFwdHost, "account.example")
			}
			if gotProto != tc.proto {
				t.Errorf("X-Forwarded-Proto = %q, want %q", gotProto, tc.proto)
			}
		})
	}
}

package handler

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter applies Redis INCR-based fixed-window limits per bucket
// (action + client IP). Cheap, no Lua needed for MVP traffic levels;
// INCR+EXPIRE races only when the window expires between calls, which
// errs on the side of allowing one extra request — acceptable here.
type RateLimiter struct {
	rdb *redis.Client
}

// NewRateLimiter wires the middleware factory.
func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{rdb: rdb}
}

// Protect returns middleware limiting `action` per client IP to n per window.
func (l *RateLimiter) Protect(action string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			key := fmt.Sprintf("rl:%s:%s", action, ip)

			n, err := l.rdb.Incr(r.Context(), key).Result()
			if err == nil && n == 1 {
				_ = l.rdb.Expire(r.Context(), key, window).Err()
			}
			if err == nil && n > int64(limit) {
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts, try again later"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP prefers the immediate peer address (no X-Forwarded-For trust —
// real IP middleware already ran; the peer is either the user or a
// reverse proxy we control).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// First hop only, if we ever sit behind our own proxy.
		if i := strings.IndexByte(fwd, ','); i > 0 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	return r.RemoteAddr
}

// Package handler wires accountd's HTTP routes.
package handler

import (
	"encoding/json"
	"net/http"
)

// Healthz reports process liveness. It does not check dependencies —
// startup already fails fast if Postgres or Redis are unreachable.
func Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

package handlers

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/iw2rmb/ploy/internal/store"
)

// healthHandler responds to health check requests, including the cluster ID when available
// from the environment (PLOY_CLUSTER_ID). It pings the database to verify connectivity.
func healthHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"status": "ok"}
		if id := os.Getenv("PLOY_CLUSTER_ID"); id != "" {
			resp["cluster_id"] = id
		}

		if err := st.Pool().Ping(r.Context()); err != nil {
			resp["status"] = "degraded"
			resp["db"] = "unreachable"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

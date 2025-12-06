package handlers

import (
	"encoding/json"
	"net/http"
	"os"
)

// healthHandler responds to health check requests, including the cluster ID when available
// from the environment (PLOY_CLUSTER_ID).
func healthHandler(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"status": "ok"}
	if id := os.Getenv("PLOY_CLUSTER_ID"); id != "" {
		resp["cluster_id"] = id
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

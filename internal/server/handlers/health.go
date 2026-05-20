package handlers

import (
	"net/http"
	"os"

	"github.com/iw2rmb/ploy/internal/store"
)

// healthzHandler responds to process liveness probes. It intentionally avoids
// dependency checks; use readyzHandler for traffic readiness.
func healthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse("ok"))
	}
}

// readyzHandler responds to readiness probes, including the cluster ID when
// available from the environment (PLOY_CLUSTER_ID). It pings the database to
// verify the control plane can serve traffic.
func readyzHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := healthResponse("ok")
		pool := st.Pool()
		if pool == nil {
			resp["status"] = "degraded"
			resp["db"] = "unreachable"
			writeJSON(w, http.StatusServiceUnavailable, resp)
			return
		}

		if err := pool.Ping(r.Context()); err != nil {
			resp["status"] = "degraded"
			resp["db"] = "unreachable"
			writeJSON(w, http.StatusServiceUnavailable, resp)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func healthResponse(status string) map[string]any {
	resp := map[string]any{"status": status}
	if id := os.Getenv("PLOY_CLUSTER_ID"); id != "" {
		resp["cluster_id"] = id
	}
	return resp
}

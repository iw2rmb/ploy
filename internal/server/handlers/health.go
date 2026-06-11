package handlers

import (
	"net/http"

	"github.com/iw2rmb/ploy/internal/store"
	iversion "github.com/iw2rmb/ploy/internal/version"
)

// healthzHandler responds to process liveness probes. It intentionally avoids
// dependency checks; use readyzHandler for traffic readiness.
func healthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse("ok"))
	}
}

// readyzHandler responds to readiness probes. It pings the database to verify
// the control plane can serve traffic.
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

		resp["db"] = "ok"
		currentVersion, err := store.CurrentSchemaVersion(r.Context(), pool)
		if err != nil {
			resp["status"] = "degraded"
			resp["db"] = "degraded"
			schema := schemaHealth()
			schema["error"] = err.Error()
			resp["schema"] = schema
			writeJSON(w, http.StatusServiceUnavailable, resp)
			return
		}

		schema := schemaHealth()
		schema["current_version"] = currentVersion
		resp["schema"] = schema
		writeJSON(w, http.StatusOK, resp)
	}
}

func healthResponse(status string) map[string]any {
	return map[string]any{
		"status": status,
		"binary": map[string]string{
			"version":  iversion.Version,
			"commit":   iversion.Commit,
			"built_at": iversion.BuiltAt,
		},
		"schema": schemaHealth(),
	}
}

func schemaHealth() map[string]any {
	return map[string]any{"target_version": store.SchemaVersion}
}

package handlers

import (
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHealthProbeHandlers(t *testing.T) {
	t.Setenv("PLOY_CLUSTER_ID", "cluster-test")

	tests := []struct {
		name       string
		path       string
		handler    http.HandlerFunc
		wantCode   int
		wantFields map[string]string
	}{
		{
			name:     "healthz_liveness",
			path:     "/healthz",
			handler:  healthzHandler(),
			wantCode: http.StatusOK,
			wantFields: map[string]string{
				"status":     "ok",
				"cluster_id": "cluster-test",
			},
		},
		{
			name:     "readyz_without_db_pool",
			path:     "/readyz",
			handler:  readyzHandler(&jobStore{}),
			wantCode: http.StatusServiceUnavailable,
			wantFields: map[string]string{
				"status":     "degraded",
				"db":         "unreachable",
				"cluster_id": "cluster-test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := doRequest(t, tt.handler, http.MethodGet, tt.path, nil)
			assertStatus(t, rr, tt.wantCode)

			body := decodeBody[map[string]any](t, rr)
			for key, want := range tt.wantFields {
				if got, ok := body[key].(string); !ok || got != want {
					t.Fatalf("%s = %#v, want %q", key, body[key], want)
				}
			}

			binary, ok := body["binary"].(map[string]any)
			if !ok {
				t.Fatalf("binary = %#v, want object", body["binary"])
			}
			if binary["version"] == "" || binary["commit"] == "" {
				t.Fatalf("binary identity is incomplete: %#v", binary)
			}

			schema, ok := body["schema"].(map[string]any)
			if !ok {
				t.Fatalf("schema = %#v, want object", body["schema"])
			}
			if got := int64(schema["target_version"].(float64)); got != store.SchemaVersion {
				t.Fatalf("schema.target_version = %d, want %d", got, store.SchemaVersion)
			}
		})
	}
}

func (m *jobStore) Pool() *pgxpool.Pool {
	return nil
}

package handlers

import (
	"net/http"
	"testing"

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

			body := decodeBody[map[string]string](t, rr)
			for key, want := range tt.wantFields {
				if body[key] != want {
					t.Fatalf("%s = %q, want %q", key, body[key], want)
				}
			}
		})
	}
}

func (m *jobStore) Pool() *pgxpool.Pool {
	return nil
}

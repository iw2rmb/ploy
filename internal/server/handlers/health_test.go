package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			rr := httptest.NewRecorder()
			tt.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rr.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantCode)
			}

			var body map[string]string
			if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
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

package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestGetJobStatusHandler(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig")

	tests := []struct {
		name           string
		storeErr       error
		overrideNodeID string // non-empty → use a different caller identity
		wantStatus     int
		wantJSON       map[string]string // nil = skip body assertions
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
			wantJSON: map[string]string{
				"job_id": f.JobID.String(),
				"status": string(domaintypes.JobStatusRunning),
			},
		},
		{
			name:           "forbidden_node_mismatch",
			overrideNodeID: domaintypes.NewNodeKey(),
			wantStatus:     http.StatusForbidden,
		},
		{
			name:       "not_found",
			storeErr:   pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "store_error",
			storeErr:   errors.New("db down"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := &jobStore{
				getJobResult: f.Job,
				getJobErr:    tt.storeErr,
			}

			handler := getJobStatusHandler(st)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.jobStatusReq(tt.overrideNodeID))

			assertStatus(t, rr, tt.wantStatus)
			for k, v := range tt.wantJSON {
				assertJSONValue(t, rr.Body.String(), k, v)
			}
		})
	}
}

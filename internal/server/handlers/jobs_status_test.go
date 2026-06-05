package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
)

func TestGetJobStatusHandler(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("mig")
	exitCode := int32(1)
	startedAt := time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(90 * time.Second)
	f.Job.JobImage = "ghcr.io/acme/mig:1"
	f.Job.ExitCode = &exitCode
	f.Job.StartedAt = pgtype.Timestamptz{Time: startedAt, Valid: true}
	f.Job.FinishedAt = pgtype.Timestamptz{Time: finishedAt, Valid: true}
	f.Job.DurationMs = 90000
	f.Job.RepoShaOut = "abcdef0123456789abcdef0123456789abcdef01"
	f.Job.RepoShaIn8 = "01234567"
	f.Job.RepoShaOut8 = "abcdef01"

	tests := []struct {
		name           string
		storeErr       error
		overrideNodeID string // non-empty → use a different caller identity
		controlPlane   bool
		omitNodeHeader bool
		wantStatus     int
		wantJSON       map[string]string // nil = skip body assertions
	}{
		{
			name:       "worker_success",
			wantStatus: http.StatusOK,
			wantJSON: map[string]string{
				"job_id":      f.JobID.String(),
				"run_id":      f.RunID.String(),
				"repo_id":     f.Job.RepoID.String(),
				"status":      string(domaintypes.JobStatusRunning),
				"job_type":    string(domaintypes.JobTypeMig),
				"job_image":   "ghcr.io/acme/mig:1",
				"repo_sha_in": f.Job.RepoShaIn,
			},
		},
		{
			name:           "control_plane_success_without_node_header",
			controlPlane:   true,
			omitNodeHeader: true,
			wantStatus:     http.StatusOK,
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

			st := &jobStore{}
			st.getJob.val = f.Job
			st.getJob.err = tt.storeErr

			handler := getJobStatusHandler(st)
			rr := httptest.NewRecorder()
			req := f.jobStatusReq(tt.overrideNodeID)
			if tt.omitNodeHeader {
				req.Header.Del(nodeUUIDHeader)
			}
			if tt.controlPlane {
				req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleControlPlane}))
			}
			handler.ServeHTTP(rr, req)

			assertStatus(t, rr, tt.wantStatus)
			for k, v := range tt.wantJSON {
				assertJSONValue(t, rr.Body.String(), k, v)
			}
		})
	}
}

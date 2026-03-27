package nodeagent

// cross_path_parity_test.go verifies that the concrete nodeagent orchestrator
// completion path (uploadFailureStatus / uploadStatus) produces status values
// consistent with lifecycle.JobStatusFromRunError for every execution outcome.
//
// This is the nodeagent-side counterpart to lifecycle.TestCrossPathTransitionParity.
// While the lifecycle fixture validates the mapping in isolation, this test
// exercises the actual runController code path so that divergence between the
// lifecycle helper and the upload call sites in execution_orchestrator*.go is
// caught immediately.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// TestRunController_UploadFailureStatus_ParityWithLifecycle calls
// uploadFailureStatus through the real runController for each execution-error
// variant and verifies that the status string sent to the server matches
// lifecycle.JobStatusFromRunError for the same error.
//
// This exercises the actual nodeagent orchestrator error path rather than
// calling the lifecycle helper in isolation.
func TestRunController_UploadFailureStatus_ParityWithLifecycle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		err        error
		wantStatus types.JobStatus
	}{
		{
			name:       "runtime error produces Fail",
			err:        errors.New("container runtime error"),
			wantStatus: types.JobStatusFail,
		},
		{
			name:       "context cancelled produces Cancelled",
			err:        context.Canceled,
			wantStatus: types.JobStatusCancelled,
		},
		{
			name:       "context deadline exceeded produces Cancelled",
			err:        context.DeadlineExceeded,
			wantStatus: types.JobStatusCancelled,
		},
		{
			name:       "wrapped context cancelled produces Cancelled",
			err:        errors.Join(errors.New("outer"), context.Canceled),
			wantStatus: types.JobStatusCancelled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Verify the lifecycle contract matches the test expectation
			// (pins this test to the same mapping as the lifecycle fixture).
			gotLifecycle := lifecycle.JobStatusFromRunError(tc.err)
			if gotLifecycle != tc.wantStatus {
				t.Fatalf(
					"lifecycle contract mismatch: JobStatusFromRunError(%v) = %v, want %v",
					tc.err, gotLifecycle, tc.wantStatus,
				)
			}

			runID := types.NewRunID()
			jobID := types.NewJobID()

			var capturedStatus string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/jobs/" + jobID.String() + "/complete":
					var payload map[string]any
					_ = json.NewDecoder(r.Body).Decode(&payload)
					if s, ok := payload["status"].(string); ok {
						capturedStatus = s
					}
					w.WriteHeader(http.StatusNoContent)
				case "/v1/nodes/" + testNodeID + "/events":
					w.WriteHeader(http.StatusCreated)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			cfg := Config{
				ServerURL: srv.URL,
				NodeID:    testNodeID,
				HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}
			rc := newTestController(t, cfg)

			req := StartRunRequest{
				RunID: runID,
				JobID: jobID,
			}
			rc.uploadFailureStatus(context.Background(), req, tc.err, 100*time.Millisecond)

			if capturedStatus != tc.wantStatus.String() {
				t.Fatalf(
					"uploadFailureStatus sent status %q, want %q (lifecycle.JobStatusFromRunError = %v)",
					capturedStatus, tc.wantStatus.String(), gotLifecycle,
				)
			}
		})
	}
}

// TestRunController_UploadStatus_SuccessAndNonZeroExit_ParityWithLifecycle
// verifies the two non-error completion paths in executeStandardJob that
// directly assign JobStatusSuccess / JobStatusFail without going through
// lifecycle.JobStatusFromRunError. These constants must remain consistent
// with the status values the lifecycle mapping expects.
func TestRunController_UploadStatus_SuccessAndNonZeroExit_ParityWithLifecycle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		status     types.JobStatus
		wantStatus string
	}{
		{
			name:       "success exit produces Success",
			status:     types.JobStatusSuccess,
			wantStatus: "Success",
		},
		{
			name:       "non-zero exit produces Fail",
			status:     types.JobStatusFail,
			wantStatus: "Fail",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runID := types.NewRunID()
			jobID := types.NewJobID()

			var capturedStatus string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/jobs/"+jobID.String()+"/complete" {
					var payload map[string]any
					_ = json.NewDecoder(r.Body).Decode(&payload)
					if s, ok := payload["status"].(string); ok {
						capturedStatus = s
					}
					w.WriteHeader(http.StatusNoContent)
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer srv.Close()

			cfg := Config{
				ServerURL: srv.URL,
				NodeID:    testNodeID,
				HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}
			rc := newTestController(t, cfg)

			var exitCode int32
			if tc.status == types.JobStatusFail {
				exitCode = 1
			}
			stats := types.NewRunStatsBuilder().ExitCode(int(exitCode)).DurationMs(100).MustBuild()
			_ = rc.uploadStatus(context.Background(), runID.String(), tc.status.String(), &exitCode, stats, jobID)

			if capturedStatus != tc.wantStatus {
				t.Fatalf("uploadStatus sent status %q, want %q", capturedStatus, tc.wantStatus)
			}
		})
	}
}

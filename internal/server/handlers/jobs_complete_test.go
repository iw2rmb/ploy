package handlers

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
)

// ===== Happy-Path & Error-Propagation Tests =====
// completeJobHandler marks a job as completed via POST /v1/jobs/{job_id}/complete.

// TestCompleteJob_Success verifies a job is completed successfully with valid payload.
func TestCompleteJob_Success(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig")
	st := newJobStoreForFixture(f)

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Success"}))

	assertStatus(t, rr, http.StatusNoContent)
	if rr.Body.Len() != 0 {
		t.Fatalf("expected empty 204 response body, got %q", rr.Body.String())
	}
	assertCalled(t, "GetJob", st.getJobCalled)
	assertCalled(t, "UpdateJobCompletion", st.updateJobCompletion.called)
}

// TestCompleteJob_WithExitCodeAndStats verifies job completion with exit_code and stats.
func TestCompleteJob_WithExitCodeAndStats(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig")
	st := newJobStoreForFixture(f)

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": int32(0),
		"stats":     map[string]any{"duration_ms": 1234},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if st.updateJobCompletion.params.ExitCode == nil {
		t.Fatal("expected exit_code to be set")
	}
	if *st.updateJobCompletion.params.ExitCode != 0 {
		t.Fatalf("expected exit_code 0, got %d", *st.updateJobCompletion.params.ExitCode)
	}
}

func TestCompleteJob_WithRepoSHAOut_Persists(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig")
	st := newJobStoreForFixture(f)

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	shaOut := "0123456789abcdef0123456789abcdef01234567"
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": shaOut,
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if st.updateJobCompletion.params.RepoShaOut != shaOut {
		t.Fatalf("expected repo_sha_out %q, got %q", shaOut, st.updateJobCompletion.params.RepoShaOut)
	}
}

func TestCompleteJob_WithJobResources_PersistsJobMetrics(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig")
	st := newJobStoreForFixture(f)

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": int32(0),
		"stats": map[string]any{
			"duration_ms": 1234,
			"job_resources": map[string]any{
				"cpu_consumed_ns":     int64(42_000_000),
				"disk_consumed_bytes": int64(512 * 1024),
				"mem_consumed_bytes":  int64(128 * 1024 * 1024),
			},
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	assertCalled(t, "UpsertJobMetric", st.upsertJobMetric.called)
	if st.upsertJobMetric.params.NodeID != f.NodeID {
		t.Fatalf("upsert node_id = %q, want %q", st.upsertJobMetric.params.NodeID, f.NodeID)
	}
	if st.upsertJobMetric.params.JobID != f.JobID {
		t.Fatalf("upsert job_id = %q, want %q", st.upsertJobMetric.params.JobID, f.JobID)
	}
	if st.upsertJobMetric.params.CpuConsumedNs != 42_000_000 {
		t.Fatalf("cpu_consumed_ns = %d, want %d", st.upsertJobMetric.params.CpuConsumedNs, int64(42_000_000))
	}
	if st.upsertJobMetric.params.DiskConsumedBytes != 512*1024 {
		t.Fatalf("disk_consumed_bytes = %d, want %d", st.upsertJobMetric.params.DiskConsumedBytes, int64(512*1024))
	}
	if st.upsertJobMetric.params.MemConsumedBytes != 128*1024*1024 {
		t.Fatalf("mem_consumed_bytes = %d, want %d", st.upsertJobMetric.params.MemConsumedBytes, int64(128*1024*1024))
	}
}

// TestCompleteJob_WithJobMetaInStats verifies that when stats.job_meta is provided,
// the handler uses UpdateJobCompletionWithMeta to persist jobs.meta JSONB.
func TestCompleteJob_WithJobMetaInStats(t *testing.T) {
	t.Parallel()

	f := newJobFixture("")
	st := newJobStoreForFixture(f)

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"duration_ms": 500,
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"log_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			},
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	assertCalled(t, "UpdateJobCompletionWithMeta", st.updateJobCompletionWithMeta.called)
	if st.updateJobCompletion.called {
		t.Fatal("did not expect UpdateJobCompletion to be called when meta is provided")
	}

	assertMetaKind(t, st.updateJobCompletionWithMeta.params.Meta, "gate")
}

// TestCompleteJob_EmptyJobMetaObjectWithWhitespaceIsIgnored verifies that an empty
// job_meta object (even if it contains whitespace like "{ }") is treated as absent.
func TestCompleteJob_EmptyJobMetaObjectWithWhitespaceIsIgnored(t *testing.T) {
	t.Parallel()

	f := newJobFixture("")
	st := newJobStoreForFixture(f)

	handler := completeJobHandler(st, nil, nil)

	// NOTE: Do not use json.Marshal here; we need whitespace inside job_meta ("{ }").
	rawBody := `{"status":"Success","exit_code":0,"stats":{"duration_ms":500,"job_meta": { } }}`

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader([]byte(rawBody)))
	req.SetPathValue("job_id", f.JobID.String())
	req.Header.Set(nodeUUIDHeader, f.NodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: f.NodeIDStr,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNoContent)
	assertCalled(t, "UpdateJobCompletion", st.updateJobCompletion.called)
	assertNotCalled(t, "UpdateJobCompletionWithMeta", st.updateJobCompletionWithMeta.called)
}

// ===== Error Propagation Tests =====

// TestCompleteJob_Exit137SetsLastError verifies that failed jobs with exit code
// 137 persist a deterministic run_repos.last_error message.
func TestCompleteJob_Exit137SetsLastError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		storeOpts []func(*jobStore)
	}{
		{name: "normal"},
		{name: "run_lookup_fails", storeOpts: []func(*jobStore){
			withGetRunErr(errors.New("transient run lookup failure")),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newRepoScopedFixture("mig")
			st := newJobStoreForFixture(f, tt.storeOpts...)

			handler := completeJobHandler(st, nil, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
				"status":    "Fail",
				"exit_code": 137,
			}))

			assertStatus(t, rr, http.StatusNoContent)
			assertCalled(t, "UpdateRunRepoError", st.updateRunRepoError.called)
		})
	}
}

// TestCompleteJob_GateFailureSetsLastError verifies that when a gate job fails
// with Stack Gate mismatch metadata, the handler sets run_repos.last_error.
func TestCompleteJob_GateFailureSetsLastError(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("pre_gate")

	st := newJobStoreForFixture(f)

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"log_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					"stack_gate": map[string]any{
						"enabled": true,
						"result":  "mismatch",
						"expected": map[string]any{
							"language": "java",
							"tool":     "maven",
							"release":  "17",
						},
						"detected": map[string]any{
							"language": "java",
							"tool":     "maven",
							"release":  "11",
						},
					},
					"log_findings": []map[string]any{
						{
							"severity": "error",
							"message":  "Stack mismatch",
							"evidence": "pom.xml: maven.compiler.release=11",
						},
					},
				},
			},
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	assertRepoError(t, st, f.RunID, f.Job.RepoID, "inbound", "Expected:", "Detected:", `release: "17"`, `release: "11"`, "Evidence:", "pom.xml")
}

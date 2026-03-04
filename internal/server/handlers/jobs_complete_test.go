package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Happy-Path & Error-Propagation Tests =====
// completeJobHandler marks a job as completed via POST /v1/jobs/{job_id}/complete.

// TestCompleteJob_Success verifies a job is completed successfully with valid payload.
func TestCompleteJob_Success(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Success"}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("expected empty 204 response body, got %q", rr.Body.String())
	}
	if !st.getJobCalled {
		t.Fatal("expected GetJob to be called")
	}
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
}

// TestCompleteJob_WithExitCodeAndStats verifies job completion with exit_code and stats.
func TestCompleteJob_WithExitCodeAndStats(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": int32(0),
		"stats":     map[string]any{"duration_ms": 1234},
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobCompletionParams.ExitCode == nil {
		t.Fatal("expected exit_code to be set")
	}
	if *st.updateJobCompletionParams.ExitCode != 0 {
		t.Fatalf("expected exit_code 0, got %d", *st.updateJobCompletionParams.ExitCode)
	}
}

func TestCompleteJob_WithRepoSHAOut_Persists(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	shaOut := "0123456789abcdef0123456789abcdef01234567"
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": shaOut,
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateJobCompletionParams.RepoShaOut != shaOut {
		t.Fatalf("expected repo_sha_out %q, got %q", shaOut, st.updateJobCompletionParams.RepoShaOut)
	}
}

func TestCompleteJob_WithJobResources_PersistsJobMetrics(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

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

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.upsertJobMetricCalled {
		t.Fatal("expected UpsertJobMetric to be called")
	}
	if st.upsertJobMetricParams.NodeID != f.NodeID {
		t.Fatalf("upsert node_id = %q, want %q", st.upsertJobMetricParams.NodeID, f.NodeID)
	}
	if st.upsertJobMetricParams.JobID != f.JobID {
		t.Fatalf("upsert job_id = %q, want %q", st.upsertJobMetricParams.JobID, f.JobID)
	}
	if st.upsertJobMetricParams.CpuConsumedNs != 42_000_000 {
		t.Fatalf("cpu_consumed_ns = %d, want %d", st.upsertJobMetricParams.CpuConsumedNs, int64(42_000_000))
	}
	if st.upsertJobMetricParams.DiskConsumedBytes != 512*1024 {
		t.Fatalf("disk_consumed_bytes = %d, want %d", st.upsertJobMetricParams.DiskConsumedBytes, int64(512*1024))
	}
	if st.upsertJobMetricParams.MemConsumedBytes != 128*1024*1024 {
		t.Fatalf("mem_consumed_bytes = %d, want %d", st.upsertJobMetricParams.MemConsumedBytes, int64(128*1024*1024))
	}
}

// TestCompleteJob_MRJobUpdatesRunStatsMRURL verifies that when an MR job
// completes with stats.metadata.mr_url, the handler merges that URL into
// runs.stats via UpdateRunStatsMRURL.
func TestCompleteJob_MRJobUpdatesRunStatsMRURL(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mr", 9000)
	mrURL := "https://gitlab.com/org/repo/-/merge_requests/42"

	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusFinished},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"duration_ms": 500,
			"metadata":    map[string]any{"mr_url": mrURL},
		},
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateRunStatsMRURLCalled {
		t.Fatal("expected UpdateRunStatsMRURL to be called")
	}
	if st.updateRunStatsMRURLParams.ID != f.RunID {
		t.Fatalf("expected UpdateRunStatsMRURL run_id %s, got %s", f.RunID, st.updateRunStatsMRURLParams.ID)
	}
	if st.updateRunStatsMRURLParams.MrUrl != mrURL {
		t.Fatalf("expected UpdateRunStatsMRURL mr_url %q, got %q", mrURL, st.updateRunStatsMRURLParams.MrUrl)
	}
}

// TestCompleteJob_WithJobMetaInStats verifies that when stats.job_meta is provided,
// the handler uses UpdateJobCompletionWithMeta to persist jobs.meta JSONB.
func TestCompleteJob_WithJobMetaInStats(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

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

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called when meta is provided")
	}

	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	if kind, ok := meta["kind"].(string); !ok || kind != "gate" {
		t.Fatalf("expected meta.kind == \"gate\", got %#v", meta["kind"])
	}
}

// TestCompleteJob_EmptyJobMetaObjectWithWhitespaceIsIgnored verifies that an empty
// job_meta object (even if it contains whitespace like "{ }") is treated as absent.
func TestCompleteJob_EmptyJobMetaObjectWithWhitespaceIsIgnored(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

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

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect UpdateJobCompletionWithMeta to be called")
	}
}

// ===== Error Propagation Tests =====

// TestCompleteJob_Exit137SetsLastError verifies that failed jobs with exit code
// 137 persist a deterministic run_repos.last_error message.
func TestCompleteJob_Exit137SetsLastError(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 2000)
	f.Job.RepoID = domaintypes.NewRepoID()

	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 137,
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateRunRepoErrorCalled {
		t.Fatal("expected UpdateRunRepoError to be called")
	}
	if st.updateRunRepoErrorParams.RunID != f.RunID {
		t.Fatalf("expected RunID %s, got %s", f.RunID, st.updateRunRepoErrorParams.RunID)
	}
	if st.updateRunRepoErrorParams.RepoID != f.Job.RepoID {
		t.Fatalf("expected RepoID %s, got %s", f.Job.RepoID, st.updateRunRepoErrorParams.RepoID)
	}
	if st.updateRunRepoErrorParams.LastError == nil {
		t.Fatal("expected LastError to be set")
	}
	msg := *st.updateRunRepoErrorParams.LastError
	for _, want := range []string{"mig-0", "exit code 137", "out of memory"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to contain %q, got: %s", want, msg)
		}
	}
}

func TestCompleteJob_Exit137SetsLastError_WhenRunLookupFails(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 2000)
	f.Job.RepoID = domaintypes.NewRepoID()

	st := &mockStore{
		getRunErr:           errors.New("transient run lookup failure"),
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 137,
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateRunRepoErrorCalled {
		t.Fatal("expected UpdateRunRepoError to be called despite run lookup failure")
	}
}

// TestCompleteJob_GateFailureSetsLastError verifies that when a gate job fails
// with Stack Gate mismatch metadata, the handler sets run_repos.last_error.
func TestCompleteJob_GateFailureSetsLastError(t *testing.T) {
	t.Parallel()

	f := newJobFixture("pre_gate", 1000)
	f.Job.RepoID = domaintypes.NewRepoID()

	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

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

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if !st.updateRunRepoErrorCalled {
		t.Fatal("expected UpdateRunRepoError to be called")
	}
	if st.updateRunRepoErrorParams.RunID != f.RunID {
		t.Fatalf("expected RunID %s, got %s", f.RunID, st.updateRunRepoErrorParams.RunID)
	}
	if st.updateRunRepoErrorParams.RepoID != f.Job.RepoID {
		t.Fatalf("expected RepoID %s, got %s", f.Job.RepoID, st.updateRunRepoErrorParams.RepoID)
	}
	if st.updateRunRepoErrorParams.LastError == nil {
		t.Fatal("expected LastError to be set")
	}

	errMsg := *st.updateRunRepoErrorParams.LastError
	for _, want := range []string{"inbound", "Expected:", "Detected:", `release: "17"`, `release: "11"`, "Evidence:", "pom.xml"} {
		if !strings.Contains(errMsg, want) {
			t.Errorf("expected error to contain %q, got: %s", want, errMsg)
		}
	}
}

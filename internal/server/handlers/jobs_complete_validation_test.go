package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

const testLogDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// ===== Input Validation & Auth Tests =====
// These tests verify request validation and authorization for completeJobHandler.

func TestCompleteJob_BadJobID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		jobID string
	}{
		{name: "missing", jobID: ""},
		{name: "whitespace", jobID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nodeID := domaintypes.NewNodeKey()
			handler := completeJobHandler(&mockStore{}, nil, nil)

			body, _ := json.Marshal(map[string]any{"status": "Success"})
			req := httptest.NewRequest(http.MethodPost, "/v1/jobs//complete", bytes.NewReader(body))
			req.SetPathValue("job_id", tt.jobID)
			req.Header.Set(nodeUUIDHeader, nodeID)
			ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
				Role: auth.RoleWorker, CommonName: nodeID,
			})
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assertStatus(t, rr, http.StatusBadRequest)
		})
	}
}

func TestCompleteJob_NoIdentity(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	handler := completeJobHandler(&mockStore{}, nil, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestCompleteJob_BadNodeHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setHeader  bool
		headerVal  string
		commonName string
	}{
		{name: "empty", setHeader: true, headerVal: "   ", commonName: "ignored"},
		{name: "invalid_format", setHeader: true, headerVal: "not-a-nanoid", commonName: "ignored"},
		{name: "missing", setHeader: false, headerVal: "", commonName: "tok_abcdef123456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture("mig", 1000)
			st := &mockStore{
				getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
				getJobResult:        f.Job,
				listJobsByRunResult: []store.Job{f.Job},
			}

			handler := completeJobHandler(st, nil, nil)

			body, _ := json.Marshal(map[string]any{"status": "Success"})
			req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
			req.SetPathValue("job_id", f.JobID.String())
			if tt.setHeader {
				req.Header.Set(nodeUUIDHeader, tt.headerVal)
			}
			ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
				Role:       auth.RoleWorker,
				CommonName: tt.commonName,
			})
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assertStatus(t, rr, http.StatusBadRequest)
			if st.updateJobCompletionCalled {
				t.Fatal("did not expect UpdateJobCompletion to be called")
			}
		})
	}
}

func TestCompleteJob_WrongNode(t *testing.T) {
	t.Parallel()

	callerNodeID := domaintypes.NewNodeKey()
	ownerNodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	job := store.Job{
		ID:     jobID,
		RunID:  runID,
		NodeID: &ownerNodeID,
		Status: domaintypes.JobStatusRunning,
		Meta:   withNextIDMeta([]byte(`{}`), 1000),
	}

	st := &mockStore{
		getJobResult:        job,
		listJobsByRunResult: []store.Job{job},
	}

	handler := completeJobHandler(st, nil, nil)

	body, _ := json.Marshal(map[string]any{"status": "Success"})
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, callerNodeID)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role: auth.RoleWorker, CommonName: callerNodeID,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusForbidden)
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called")
	}
}

func TestCompleteJob_NonRunningConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		jobStatus domaintypes.JobStatus
	}{
		{name: "created", jobStatus: domaintypes.JobStatusCreated},
		{name: "success", jobStatus: domaintypes.JobStatusSuccess},
		{name: "fail", jobStatus: domaintypes.JobStatusFail},
		{name: "cancelled", jobStatus: domaintypes.JobStatusCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture("mig", 1000)
			f.Job.Status = tt.jobStatus

			st := &mockStore{
				getJobResult:        f.Job,
				listJobsByRunResult: []store.Job{f.Job},
			}

			handler := completeJobHandler(st, nil, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

			assertStatus(t, rr, http.StatusConflict)
			if st.updateJobCompletionCalled {
				t.Fatal("did not expect UpdateJobCompletion to be called")
			}
			if st.updateJobCompletionWithMetaCalled {
				t.Fatal("did not expect UpdateJobCompletionWithMeta to be called")
			}
		})
	}
}

func TestCompleteJob_InvalidStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	handler := completeJobHandler(&mockStore{}, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "running"}))

	assertStatus(t, rr, http.StatusBadRequest)
}

func TestCompleteJob_InvalidRepoSHAOut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sha  string
	}{
		{name: "uppercase", sha: "0123456789ABCDEF0123456789ABCDEF01234567"},
		{name: "too_short", sha: "0123456789abcdef"},
		{name: "non_hex", sha: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
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
				"status":       "Success",
				"repo_sha_out": tt.sha,
			}))

			assertStatus(t, rr, http.StatusBadRequest)
			if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
				t.Fatal("did not expect completion persistence on invalid repo_sha_out")
			}
		})
	}
}

func TestCompleteJob_MissingRepoSHAOutForSuccessfulLinkedJob(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	nextID := domaintypes.NewJobID()
	f.Job.NextID = &nextID
	f.Job.RepoShaIn = "0123456789abcdef0123456789abcdef01234567"

	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status": "Success",
	}))

	assertStatus(t, rr, http.StatusBadRequest)
	if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect completion persistence when repo_sha_out is missing")
	}
}

func TestCompleteJob_InvalidRepoSHAInForSuccessfulLinkedJob(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	nextID := domaintypes.NewJobID()
	f.Job.NextID = &nextID
	f.Job.RepoShaIn = ""

	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	assertStatus(t, rr, http.StatusConflict)
	if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect completion persistence when repo_sha_in is invalid")
	}
}

func TestCompleteJob_MissingStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	handler := completeJobHandler(&mockStore{}, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{}))

	assertStatus(t, rr, http.StatusBadRequest)
}

func TestCompleteJob_StatsMustBeObject(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status": "Fail",
		"stats":  "oops",
	}))

	assertStatus(t, rr, http.StatusBadRequest)
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called")
	}
}

func TestCompleteJob_JobNotFound(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 1000)
	st := &mockStore{getJobErr: pgx.ErrNoRows}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

	assertStatus(t, rr, http.StatusNotFound)
}

func TestCompleteJob_InvalidJobResources_RejectsRequest(t *testing.T) {
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
		"status": "Success",
		"stats": map[string]any{
			"job_resources": map[string]any{
				"cpu_consumed_ns": -1,
			},
		},
	}))

	assertStatus(t, rr, http.StatusBadRequest)
	if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect job completion updates on invalid job_resources")
	}
	if st.upsertJobMetricCalled {
		t.Fatal("did not expect UpsertJobMetric on invalid job_resources")
	}
}

// ===== JobMeta Validation Tests =====

func newCompleteJobMetaFixture() (jobTestFixture, *mockStore, http.Handler) {
	f := newJobFixture("", 1000)
	st := newMockStoreForJob(f)
	return f, st, completeJobHandler(st, nil, nil)
}

func TestCompleteJob_InvalidJobMeta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		jobMeta            map[string]any
		expectBodyContains string
	}{
		{
			name: "missing_kind",
			jobMeta: map[string]any{
				"gate": map[string]any{"log_digest": testLogDigest},
			},
			expectBodyContains: "job_meta",
		},
		{
			name: "invalid_kind",
			jobMeta: map[string]any{
				"kind": "invalid_kind",
			},
			expectBodyContains: "job_meta",
		},
		{
			name: "gate_meta_on_mig_kind",
			jobMeta: map[string]any{
				"kind": "mig",
				"gate": map[string]any{"log_digest": testLogDigest},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, st, handler := newCompleteJobMetaFixture()

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
				"status":    "Success",
				"exit_code": 0,
				"stats": map[string]any{
					"job_meta": tt.jobMeta,
				},
			}))

			assertStatus(t, rr, http.StatusBadRequest)
			if tt.expectBodyContains != "" && !strings.Contains(rr.Body.String(), tt.expectBodyContains) {
				t.Fatalf("expected response body to contain %q, got: %s", tt.expectBodyContains, rr.Body.String())
			}
			if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
				t.Fatal("did not expect job completion to be called for invalid job_meta")
			}
		})
	}
}

func TestCompleteJob_ValidJobMetaKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		jobMeta      map[string]any
		expectedKind string
	}{
		{
			name: "gate",
			jobMeta: map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"log_digest": testLogDigest,
					"static_checks": []map[string]any{
						{"tool": "maven", "passed": true},
					},
				},
			},
			expectedKind: "gate",
		},
		{
			name: "mig",
			jobMeta: map[string]any{
				"kind": "mig",
			},
			expectedKind: "mig",
		},
		{
			name: "build",
			jobMeta: map[string]any{
				"kind": "build",
				"build": map[string]any{
					"tool":    "maven",
					"command": "mvn clean install",
				},
			},
			expectedKind: "build",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, st, handler := newCompleteJobMetaFixture()

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
				"status":    "Success",
				"exit_code": 0,
				"stats": map[string]any{
					"job_meta": tt.jobMeta,
				},
			}))

			assertStatus(t, rr, http.StatusNoContent)
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
			if kind, ok := meta["kind"].(string); !ok || kind != tt.expectedKind {
				t.Fatalf("expected meta.kind == %q, got %#v", tt.expectedKind, meta["kind"])
			}
		})
	}
}

func TestCompleteJob_EmptyJobMeta_NoPersist(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 2000)
	st := newMockStoreForJob(f)
	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta":    map[string]any{},
			"duration_ms": 500,
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect UpdateJobCompletionWithMeta for empty job_meta")
	}
}

func TestCompleteJob_NullJobMeta_NoPersist(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 2000)
	st := newMockStoreForJob(f)
	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta":    nil,
			"duration_ms": 500,
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect UpdateJobCompletionWithMeta for null job_meta")
	}
}

func TestCompleteJob_ValidJobMeta_GateWithBugSummary(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)
	st := newMockStoreForJob(f)
	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"log_digest":  testLogDigest,
					"bug_summary": "Missing semicolon on line 42 of Main.java",
				},
			},
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	gate, ok := meta["gate"].(map[string]any)
	if !ok {
		t.Fatal("expected gate metadata to be present")
	}
	if bs, ok := gate["bug_summary"].(string); !ok || bs != "Missing semicolon on line 42 of Main.java" {
		t.Fatalf("expected bug_summary = %q, got %#v", "Missing semicolon on line 42 of Main.java", gate["bug_summary"])
	}
}

func TestCompleteJob_ValidJobMeta_ModWithActionSummary(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 2000)
	st := newMockStoreForJob(f)
	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind":           "mig",
				"action_summary": "Fixed missing import in Main.java",
			},
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	if as, ok := meta["action_summary"].(string); !ok || as != "Fixed missing import in Main.java" {
		t.Fatalf("expected action_summary = %q, got %#v", "Fixed missing import in Main.java", meta["action_summary"])
	}
}

// ===== JobStatsPayload Unit Tests =====

func TestJobStatsPayload_MRURL(t *testing.T) {
	tests := []struct {
		name     string
		payload  JobStatsPayload
		expected string
	}{
		{name: "nil metadata", payload: JobStatsPayload{}, expected: ""},
		{name: "empty metadata", payload: JobStatsPayload{Metadata: map[string]string{}}, expected: ""},
		{name: "mr_url present", payload: JobStatsPayload{Metadata: map[string]string{"mr_url": "https://gitlab.com/mr/1"}}, expected: "https://gitlab.com/mr/1"},
		{name: "mr_url with whitespace", payload: JobStatsPayload{Metadata: map[string]string{"mr_url": "  https://gitlab.com/mr/2  "}}, expected: "https://gitlab.com/mr/2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.payload.MRURL(); got != tc.expected {
				t.Errorf("MRURL() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestJobStatsPayload_HasJobMeta(t *testing.T) {
	tests := []struct {
		name     string
		payload  JobStatsPayload
		expected bool
	}{
		{name: "nil job_meta", payload: JobStatsPayload{}, expected: false},
		{name: "empty job_meta bytes", payload: JobStatsPayload{JobMeta: []byte{}}, expected: false},
		{name: "empty object job_meta", payload: JobStatsPayload{JobMeta: []byte("{}")}, expected: false},
		{name: "empty object with whitespace", payload: JobStatsPayload{JobMeta: []byte("{ }")}, expected: false},
		{name: "null job_meta", payload: JobStatsPayload{JobMeta: []byte("null")}, expected: false},
		{name: "valid job_meta", payload: JobStatsPayload{JobMeta: []byte(`{"kind":"mig"}`)}, expected: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.payload.HasJobMeta(); got != tc.expected {
				t.Errorf("HasJobMeta() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestJobStatsPayload_ValidateJobMeta(t *testing.T) {
	tests := []struct {
		name    string
		payload JobStatsPayload
		wantErr bool
	}{
		{name: "nil job_meta", payload: JobStatsPayload{}, wantErr: false},
		{name: "empty job_meta", payload: JobStatsPayload{JobMeta: []byte("{}")}, wantErr: false},
		{name: "empty with whitespace", payload: JobStatsPayload{JobMeta: []byte("{ }")}, wantErr: false},
		{name: "null job_meta", payload: JobStatsPayload{JobMeta: []byte("null")}, wantErr: false},
		{name: "valid mig kind", payload: JobStatsPayload{JobMeta: []byte(`{"kind":"mig"}`)}, wantErr: false},
		{name: "valid gate kind", payload: JobStatsPayload{JobMeta: []byte("{\"kind\":\"gate\",\"gate\":{\"log_digest\":\"" + testLogDigest + "\"}}")}, wantErr: false},
		{name: "valid build kind", payload: JobStatsPayload{JobMeta: []byte(`{"kind":"build","build":{"tool":"maven"}}`)}, wantErr: false},
		{name: "missing kind field", payload: JobStatsPayload{JobMeta: []byte("{\"gate\":{\"log_digest\":\"" + testLogDigest + "\"}}")}, wantErr: true},
		{name: "invalid kind value", payload: JobStatsPayload{JobMeta: []byte(`{"kind":"unknown"}`)}, wantErr: true},
		{name: "gate metadata on mig kind", payload: JobStatsPayload{JobMeta: []byte("{\"kind\":\"mig\",\"gate\":{\"log_digest\":\"" + testLogDigest + "\"}}")}, wantErr: true},
		{name: "invalid json", payload: JobStatsPayload{JobMeta: []byte(`{invalid}`)}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.payload.ValidateJobMeta()
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateJobMeta() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

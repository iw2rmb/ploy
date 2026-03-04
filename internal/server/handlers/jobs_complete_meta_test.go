package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

const testLogDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// ===== JobMeta Validation Tests =====
// These tests verify that job_meta payloads are validated via contracts.UnmarshalJobMeta
// before persisting to jobs.meta JSONB.

func newCompleteJobMetaFixture() (jobTestFixture, *mockStore, http.Handler) {
	f := newJobFixture("", 1000)
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}
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

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
			}
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
			if kind, ok := meta["kind"].(string); !ok || kind != tt.expectedKind {
				t.Fatalf("expected meta.kind == %q, got %#v", tt.expectedKind, meta["kind"])
			}
		})
	}
}

// TestCompleteJob_EmptyJobMeta_NoPersist verifies empty job_meta values don't trigger
// UpdateJobCompletionWithMeta (use regular UpdateJobCompletion instead).
func TestCompleteJob_EmptyJobMeta_NoPersist(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 2000)

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)

	// Empty job_meta object should NOT trigger UpdateJobCompletionWithMeta.
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta":    map[string]any{}, // Empty object
			"duration_ms": 500,
		},
	}))

	// Expect 204 No Content.
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify UpdateJobCompletion was called (not WithMeta variant).
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect UpdateJobCompletionWithMeta for empty job_meta")
	}
}

// TestCompleteJob_NullJobMeta_NoPersist verifies null job_meta values don't trigger
// UpdateJobCompletionWithMeta.
func TestCompleteJob_NullJobMeta_NoPersist(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 2000)

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)

	// Null job_meta should NOT trigger UpdateJobCompletionWithMeta.
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats": map[string]any{
			"job_meta":    nil,
			"duration_ms": 500,
		},
	}))

	// Expect 204 No Content.
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify UpdateJobCompletion was called (not WithMeta variant).
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect UpdateJobCompletionWithMeta for null job_meta")
	}
}

// TestCompleteJob_ValidJobMeta_GateWithBugSummary verifies that gate job_meta with
// bug_summary is accepted and persisted.
func TestCompleteJob_ValidJobMeta_GateWithBugSummary(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)

	// Gate job_meta with bug_summary in gate metadata.
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

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	// Verify bug_summary is persisted in gate metadata.
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

// TestCompleteJob_ValidJobMeta_ModWithActionSummary verifies that mig job_meta
// with action_summary is accepted and persisted.
func TestCompleteJob_ValidJobMeta_ModWithActionSummary(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 2000)

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)

	// Mig job_meta with action_summary.
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

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}
	// Verify action_summary is persisted in meta.
	var meta map[string]any
	if err := json.Unmarshal(st.updateJobCompletionWithMetaParams.Meta, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	if as, ok := meta["action_summary"].(string); !ok || as != "Fixed missing import in Main.java" {
		t.Fatalf("expected action_summary = %q, got %#v", "Fixed missing import in Main.java", meta["action_summary"])
	}
}

// ===== JobStatsPayload Unit Tests =====
// These tests verify the typed JobStatsPayload struct behavior.

func TestJobStatsPayload_MRURL(t *testing.T) {
	tests := []struct {
		name     string
		payload  JobStatsPayload
		expected string
	}{
		{
			name:     "nil metadata",
			payload:  JobStatsPayload{},
			expected: "",
		},
		{
			name:     "empty metadata",
			payload:  JobStatsPayload{Metadata: map[string]string{}},
			expected: "",
		},
		{
			name:     "mr_url present",
			payload:  JobStatsPayload{Metadata: map[string]string{"mr_url": "https://gitlab.com/mr/1"}},
			expected: "https://gitlab.com/mr/1",
		},
		{
			name:     "mr_url with whitespace",
			payload:  JobStatsPayload{Metadata: map[string]string{"mr_url": "  https://gitlab.com/mr/2  "}},
			expected: "https://gitlab.com/mr/2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.payload.MRURL()
			if got != tc.expected {
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
		{
			name:     "nil job_meta",
			payload:  JobStatsPayload{},
			expected: false,
		},
		{
			name:     "empty job_meta bytes",
			payload:  JobStatsPayload{JobMeta: []byte{}},
			expected: false,
		},
		{
			name:     "empty object job_meta",
			payload:  JobStatsPayload{JobMeta: []byte("{}")},
			expected: false,
		},
		{
			name:     "empty object job_meta with whitespace",
			payload:  JobStatsPayload{JobMeta: []byte("{ }")},
			expected: false,
		},
		{
			name:     "null job_meta",
			payload:  JobStatsPayload{JobMeta: []byte("null")},
			expected: false,
		},
		{
			name:     "valid job_meta",
			payload:  JobStatsPayload{JobMeta: []byte(`{"kind":"mig"}`)},
			expected: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.payload.HasJobMeta()
			if got != tc.expected {
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
		{
			name:    "nil job_meta",
			payload: JobStatsPayload{},
			wantErr: false, // No job_meta is valid (optional).
		},
		{
			name:    "empty job_meta",
			payload: JobStatsPayload{JobMeta: []byte("{}")},
			wantErr: false, // Empty is treated as "no job_meta".
		},
		{
			name:    "empty job_meta with whitespace",
			payload: JobStatsPayload{JobMeta: []byte("{ }")},
			wantErr: false, // Empty is treated as "no job_meta".
		},
		{
			name:    "null job_meta",
			payload: JobStatsPayload{JobMeta: []byte("null")},
			wantErr: false, // Null is treated as "no job_meta".
		},
		{
			name:    "valid mig kind",
			payload: JobStatsPayload{JobMeta: []byte(`{"kind":"mig"}`)},
			wantErr: false,
		},
		{
			name:    "valid gate kind",
			payload: JobStatsPayload{JobMeta: []byte("{\"kind\":\"gate\",\"gate\":{\"log_digest\":\"" + testLogDigest + "\"}}")},
			wantErr: false,
		},
		{
			name:    "valid build kind",
			payload: JobStatsPayload{JobMeta: []byte(`{"kind":"build","build":{"tool":"maven"}}`)},
			wantErr: false,
		},
		{
			name:    "missing kind field",
			payload: JobStatsPayload{JobMeta: []byte("{\"gate\":{\"log_digest\":\"" + testLogDigest + "\"}}")},
			wantErr: true,
		},
		{
			name:    "invalid kind value",
			payload: JobStatsPayload{JobMeta: []byte(`{"kind":"unknown"}`)},
			wantErr: true,
		},
		{
			name:    "gate metadata on mig kind",
			payload: JobStatsPayload{JobMeta: []byte("{\"kind\":\"mig\",\"gate\":{\"log_digest\":\"" + testLogDigest + "\"}}")},
			wantErr: true,
		},
		{
			name:    "invalid json",
			payload: JobStatsPayload{JobMeta: []byte(`{invalid}`)},
			wantErr: true,
		},
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

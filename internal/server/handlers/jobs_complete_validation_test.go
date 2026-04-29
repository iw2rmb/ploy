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

func TestCompleteJob_RequestRejection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		buildReq   func(f jobTestFixture) *http.Request
		wantStatus int
	}{
		{
			name: "bad_job_id/missing",
			buildReq: func(f jobTestFixture) *http.Request {
				body, _ := json.Marshal(map[string]any{"status": "Success"})
				req := httptest.NewRequest(http.MethodPost, "/v1/jobs//complete", bytes.NewReader(body))
				req.SetPathValue("job_id", "")
				req.Header.Set(nodeUUIDHeader, f.NodeIDStr)
				ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: f.NodeIDStr})
				return req.WithContext(ctx)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "bad_job_id/whitespace",
			buildReq: func(f jobTestFixture) *http.Request {
				body, _ := json.Marshal(map[string]any{"status": "Success"})
				req := httptest.NewRequest(http.MethodPost, "/v1/jobs//complete", bytes.NewReader(body))
				req.SetPathValue("job_id", "   ")
				req.Header.Set(nodeUUIDHeader, f.NodeIDStr)
				ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: f.NodeIDStr})
				return req.WithContext(ctx)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "no_identity",
			buildReq: func(f jobTestFixture) *http.Request {
				body, _ := json.Marshal(map[string]any{"status": "Success"})
				req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
				req.SetPathValue("job_id", f.JobID.String())
				return req
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "bad_node_header/empty",
			buildReq: func(f jobTestFixture) *http.Request {
				body, _ := json.Marshal(map[string]any{"status": "Success"})
				req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
				req.SetPathValue("job_id", f.JobID.String())
				req.Header.Set(nodeUUIDHeader, "   ")
				ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: "ignored"})
				return req.WithContext(ctx)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "bad_node_header/invalid_format",
			buildReq: func(f jobTestFixture) *http.Request {
				body, _ := json.Marshal(map[string]any{"status": "Success"})
				req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
				req.SetPathValue("job_id", f.JobID.String())
				req.Header.Set(nodeUUIDHeader, "not-a-nanoid")
				ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: "ignored"})
				return req.WithContext(ctx)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "bad_node_header/missing",
			buildReq: func(f jobTestFixture) *http.Request {
				body, _ := json.Marshal(map[string]any{"status": "Success"})
				req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
				req.SetPathValue("job_id", f.JobID.String())
				ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: "tok_abcdef123456"})
				return req.WithContext(ctx)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "wrong_node",
			buildReq: func(f jobTestFixture) *http.Request {
				callerNodeID := domaintypes.NewNodeKey()
				req := f.completeJobReq(map[string]any{"status": "Success"})
				req.Header.Set(nodeUUIDHeader, callerNodeID)
				ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: callerNodeID})
				return req.WithContext(ctx)
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture("mig")
			st := newJobStoreForFixture(f)
			handler := completeJobHandler(st, nil, nil, nil)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, tt.buildReq(f))

			assertStatus(t, rr, tt.wantStatus)
			assertNoCompletion(t, st)
		})
	}
}

// ===== Payload & State Rejection Tests =====

func TestCompleteJob_PayloadRejection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tweakJob   func(*store.Job)
		storeOpts  []func(*jobStore)
		body       map[string]any
		wantStatus int
	}{
		{name: "missing_status", body: map[string]any{}, wantStatus: http.StatusBadRequest},
		{name: "invalid_status", body: map[string]any{"status": "running"}, wantStatus: http.StatusBadRequest},
		{name: "stats_not_object", body: map[string]any{"status": "Fail", "stats": "oops"}, wantStatus: http.StatusBadRequest},
		{name: "repo_sha_out/uppercase", body: map[string]any{"status": "Success", "repo_sha_out": "0123456789ABCDEF0123456789ABCDEF01234567"}, wantStatus: http.StatusBadRequest},
		{name: "repo_sha_out/too_short", body: map[string]any{"status": "Success", "repo_sha_out": "0123456789abcdef"}, wantStatus: http.StatusBadRequest},
		{name: "repo_sha_out/non_hex", body: map[string]any{"status": "Success", "repo_sha_out": "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}, wantStatus: http.StatusBadRequest},
		{
			name: "linked_job/missing_sha_out",
			tweakJob: func(j *store.Job) {
				nextID := domaintypes.NewJobID()
				j.NextID = &nextID
				j.RepoShaIn = "0123456789abcdef0123456789abcdef01234567"
			},
			body:       map[string]any{"status": "Success"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "linked_job/empty_sha_in",
			tweakJob: func(j *store.Job) {
				nextID := domaintypes.NewJobID()
				j.NextID = &nextID
				j.RepoShaIn = ""
			},
			body:       map[string]any{"status": "Success", "repo_sha_out": "0123456789abcdef0123456789abcdef01234567"},
			wantStatus: http.StatusConflict,
		},
		{name: "conflict/created", tweakJob: func(j *store.Job) { j.Status = domaintypes.JobStatusCreated }, body: map[string]any{"status": "Fail"}, wantStatus: http.StatusConflict},
		{name: "conflict/success", tweakJob: func(j *store.Job) { j.Status = domaintypes.JobStatusSuccess }, body: map[string]any{"status": "Fail"}, wantStatus: http.StatusConflict},
		{name: "conflict/fail", tweakJob: func(j *store.Job) { j.Status = domaintypes.JobStatusFail }, body: map[string]any{"status": "Fail"}, wantStatus: http.StatusConflict},
		{name: "conflict/cancelled", tweakJob: func(j *store.Job) { j.Status = domaintypes.JobStatusCancelled }, body: map[string]any{"status": "Fail"}, wantStatus: http.StatusConflict},
		{
			name:       "job_not_found",
			storeOpts:  []func(*jobStore){withGetJobErr(pgx.ErrNoRows)},
			body:       map[string]any{"status": "Fail"},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "invalid_job_resources",
			body: map[string]any{
				"status": "Success",
				"stats":  map[string]any{"job_resources": map[string]any{"cpu_consumed_ns": -1}},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture("mig")
			if tt.tweakJob != nil {
				tt.tweakJob(&f.Job)
			}
			st := newJobStoreForFixture(f, tt.storeOpts...)
			handler := completeJobHandler(st, nil, nil, nil)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(tt.body))

			assertStatus(t, rr, tt.wantStatus)
			assertNoCompletion(t, st)
		})
	}
}

// ===== JobMeta Validation Tests =====

func TestCompleteJob_InvalidJobMeta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		jobMeta            map[string]any
		expectBodyContains string
	}{
		{
			name:               "missing_kind",
			jobMeta:            map[string]any{"gate": map[string]any{"log_digest": testLogDigest}},
			expectBodyContains: "job_meta",
		},
		{
			name:               "invalid_kind",
			jobMeta:            map[string]any{"kind": "invalid_kind"},
			expectBodyContains: "job_meta",
		},
		{name: "gate_meta_on_mig_kind", jobMeta: map[string]any{"kind": "mig", "gate": map[string]any{"log_digest": testLogDigest}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture("")
			st := newJobStoreForFixture(f)
			handler := completeJobHandler(st, nil, nil, nil)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
				"status": "Success", "exit_code": 0,
				"stats": map[string]any{"job_meta": tt.jobMeta},
			}))

			assertStatus(t, rr, http.StatusBadRequest)
			if tt.expectBodyContains != "" && !strings.Contains(rr.Body.String(), tt.expectBodyContains) {
				t.Fatalf("expected response body to contain %q, got: %s", tt.expectBodyContains, rr.Body.String())
			}
			assertNoCompletion(t, st)
		})
	}
}

// ===== Valid Completion Tests =====

func TestCompleteJob_ValidCompletion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         map[string]any
		wantWithMeta bool
		wantMetaKind string
		checkMeta    func(t *testing.T, meta map[string]any)
	}{
		// Absent/empty meta -> plain completion path
		{
			name:         "absent_meta/empty",
			body:         map[string]any{"status": "Success", "exit_code": 0, "stats": map[string]any{"job_meta": map[string]any{}, "duration_ms": 500}},
			wantWithMeta: false,
		},
		{
			name:         "absent_meta/null",
			body:         map[string]any{"status": "Success", "exit_code": 0, "stats": map[string]any{"job_meta": nil, "duration_ms": 500}},
			wantWithMeta: false,
		},
		// Valid meta kinds -> completion with meta
		{
			name:         "meta_kind/gate",
			wantWithMeta: true, wantMetaKind: "gate",
			body: map[string]any{"status": "Success", "exit_code": 0, "stats": map[string]any{
				"job_meta": map[string]any{"kind": "gate", "gate": map[string]any{
					"log_digest":    testLogDigest,
					"static_checks": []map[string]any{{"tool": "maven", "passed": true}},
				}},
			}},
		},
		{
			name:         "meta_kind/mig",
			wantWithMeta: true, wantMetaKind: "mig",
			body: map[string]any{"status": "Success", "exit_code": 0, "stats": map[string]any{
				"job_meta": map[string]any{"kind": "mig"},
			}},
		},
		{
			name:         "meta_kind/build",
			wantWithMeta: true, wantMetaKind: "build",
			body: map[string]any{"status": "Success", "exit_code": 0, "stats": map[string]any{
				"job_meta": map[string]any{"kind": "build", "build": map[string]any{"tool": "maven", "command": "mvn clean install"}},
			}},
		},
		// Field persistence
		{
			name:         "field_persistence/gate_bug_summary",
			wantWithMeta: true, wantMetaKind: "gate",
			body: map[string]any{"status": "Fail", "exit_code": 1, "stats": map[string]any{
				"job_meta": map[string]any{"kind": "gate", "gate": map[string]any{
					"log_digest":  testLogDigest,
					"bug_summary": "Missing semicolon on line 42 of Main.java",
				}},
			}},
			checkMeta: func(t *testing.T, meta map[string]any) {
				gate, ok := meta["gate"].(map[string]any)
				if !ok {
					t.Fatal("expected gate metadata to be present")
				}
				if bs, ok := gate["bug_summary"].(string); !ok || bs != "Missing semicolon on line 42 of Main.java" {
					t.Fatalf("expected bug_summary = %q, got %#v", "Missing semicolon on line 42 of Main.java", gate["bug_summary"])
				}
			},
		},
		{
			name:         "field_persistence/mig_action_summary",
			wantWithMeta: true, wantMetaKind: "mig",
			body: map[string]any{"status": "Success", "exit_code": 0, "stats": map[string]any{
				"job_meta": map[string]any{"kind": "mig", "action_summary": "Fixed missing import in Main.java"},
			}},
			checkMeta: func(t *testing.T, meta map[string]any) {
				if as, ok := meta["action_summary"].(string); !ok || as != "Fixed missing import in Main.java" {
					t.Fatalf("expected action_summary = %q, got %#v", "Fixed missing import in Main.java", meta["action_summary"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture("")
			st := newJobStoreForFixture(f)
			handler := completeJobHandler(st, nil, nil, nil)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(tt.body))

			assertStatus(t, rr, http.StatusNoContent)

			if tt.wantWithMeta {
				assertCalled(t, "UpdateJobCompletionWithMeta", st.updateJobCompletionWithMeta.called)
				assertNotCalled(t, "UpdateJobCompletion", st.updateJobCompletion.called)
				assertMetaKind(t, st.updateJobCompletionWithMeta.params.Meta, tt.wantMetaKind)
				if tt.checkMeta != nil {
					var meta map[string]any
					if err := json.Unmarshal(st.updateJobCompletionWithMeta.params.Meta, &meta); err != nil {
						t.Fatalf("failed to unmarshal persisted meta: %v", err)
					}
					tt.checkMeta(t, meta)
				}
			} else {
				assertCalled(t, "UpdateJobCompletion", st.updateJobCompletion.called)
				assertNotCalled(t, "UpdateJobCompletionWithMeta", st.updateJobCompletionWithMeta.called)
			}
		})
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

func TestJobStatsPayload_ErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		payload  JobStatsPayload
		expected string
	}{
		{name: "empty", payload: JobStatsPayload{}, expected: ""},
		{name: "trimmed", payload: JobStatsPayload{Error: "  gate execution failed: network missing  "}, expected: "gate execution failed: network missing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.payload.ErrorMessage(); got != tc.expected {
				t.Errorf("ErrorMessage() = %q, want %q", got, tc.expected)
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

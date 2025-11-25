package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

func TestValidateBuildGate_SyncCompletionHappyPath(t *testing.T) {
	st := &mockStore{}

	// Seed CreateBuildGateJob (pending)
	jobID := uuid.New()
	st.createBGJobResult = store.BuildgateJob{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		Status:    store.BuildgateJobStatusPending,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	// Seed GetBuildGateJob to return completed with a minimal result
	st.getBGJobResult = store.BuildgateJob{
		ID:        pgtype.UUID{Bytes: jobID, Valid: true},
		Status:    store.BuildgateJobStatusCompleted,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	// Minimal result payload: {"static_checks":[{"tool":"t","passed":true}]}
	st.getBGJobResult.Result = []byte(`{"static_checks":[{"tool":"t","passed":true}]}`)

	body := map[string]any{
		"repo_url": "https://example.com/repo.git",
		"ref":      "main",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/buildgate/validate", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h := validateBuildGateHandler(st)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		JobID  string          `json:"job_id"`
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.JobID == "" || resp.Status != "completed" || len(resp.Result) == 0 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

// TestValidateBuildGate_MissingRepoURL verifies that requests without repo_url are rejected.
func TestValidateBuildGate_MissingRepoURL(t *testing.T) {
	st := &mockStore{}
	body := map[string]any{
		"ref": "main",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/buildgate/validate", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h := validateBuildGateHandler(st)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "repo_url is required") {
		t.Fatalf("expected error about missing repo_url, got: %s", rr.Body.String())
	}
}

// TestValidateBuildGate_MissingRef verifies that requests without ref are rejected.
func TestValidateBuildGate_MissingRef(t *testing.T) {
	st := &mockStore{}
	body := map[string]any{
		"repo_url": "https://example.com/repo.git",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/buildgate/validate", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h := validateBuildGateHandler(st)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "ref is required") {
		t.Fatalf("expected error about missing ref, got: %s", rr.Body.String())
	}
}

// TestValidateBuildGate_EmptyRequest verifies that empty requests are rejected.
func TestValidateBuildGate_EmptyRequest(t *testing.T) {
	st := &mockStore{}
	body := map[string]any{}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/buildgate/validate", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h := validateBuildGateHandler(st)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "repo_url is required") {
		t.Fatalf("expected error about missing repo_url, got: %s", rr.Body.String())
	}
}

// TestValidateBuildGate_LegacyArchivePayloadRejected verifies that legacy
// archive-style payloads that omit repo_url/ref are rejected.
func TestValidateBuildGate_LegacyArchivePayloadRejected(t *testing.T) {
	st := &mockStore{}
	// Legacy payload with archive-style content but no repo_url+ref.
	// Since archive uploads are no longer recognized, the request should fail
	// validation because repo_url and ref are now required.
	body := map[string]any{
		"legacy_archive": "SGVsbG8gV29ybGQ=", // base64 "Hello World"
		"profile":        "java-maven",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/buildgate/validate", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h := validateBuildGateHandler(st)
	h.ServeHTTP(rr, req)

	// Should be rejected because repo_url is now required.
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for legacy archive-style payload, got status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "repo_url is required") {
		t.Fatalf("expected error about missing repo_url, got: %s", rr.Body.String())
	}
}

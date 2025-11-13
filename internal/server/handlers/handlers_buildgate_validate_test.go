package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

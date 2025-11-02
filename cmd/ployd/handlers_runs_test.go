package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestCreateRunHandlerSuccess verifies successful run creation.
func TestCreateRunHandlerSuccess(t *testing.T) {
	runID := uuid.New()
	modID := uuid.New()
	now := time.Now()

	st := &mockStore{
		createRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			ModID:     pgtype.UUID{Bytes: modID, Valid: true},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CommitSha: strPtr("abc123"),
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := createRunHandler(st)

	reqBody := map[string]interface{}{
		"mod_id":     modID.String(),
		"base_ref":   "main",
		"target_ref": "feature",
		"commit_sha": "abc123",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.RunID != runID.String() {
		t.Errorf("expected run_id %s, got %s", runID.String(), resp.RunID)
	}

	if !st.createRunCalled {
		t.Error("expected CreateRun to be called")
	}

	// Verify status was set to queued.
	if st.createRunParams.Status != store.RunStatusQueued {
		t.Errorf("expected status queued, got %s", st.createRunParams.Status)
	}
}

// TestCreateRunHandlerMissingFields verifies that missing required fields are rejected.
func TestCreateRunHandlerMissingFields(t *testing.T) {
	st := &mockStore{}
	handler := createRunHandler(st)

	cases := []struct {
		name     string
		body     map[string]interface{}
		wantErr  string
		wantCode int
	}{
		{
			name:     "missing mod_id",
			body:     map[string]interface{}{"base_ref": "main", "target_ref": "feature"},
			wantErr:  "mod_id field is required",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty mod_id",
			body:     map[string]interface{}{"mod_id": "", "base_ref": "main", "target_ref": "feature"},
			wantErr:  "mod_id field is required",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid mod_id format",
			body:     map[string]interface{}{"mod_id": "not-a-uuid", "base_ref": "main", "target_ref": "feature"},
			wantErr:  "invalid mod_id",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing base_ref",
			body:     map[string]interface{}{"mod_id": uuid.New().String(), "target_ref": "feature"},
			wantErr:  "base_ref field is required",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty base_ref",
			body:     map[string]interface{}{"mod_id": uuid.New().String(), "base_ref": "", "target_ref": "feature"},
			wantErr:  "base_ref field is required",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing target_ref",
			body:     map[string]interface{}{"mod_id": uuid.New().String(), "base_ref": "main"},
			wantErr:  "target_ref field is required",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty target_ref",
			body:     map[string]interface{}{"mod_id": uuid.New().String(), "base_ref": "main", "target_ref": ""},
			wantErr:  "target_ref field is required",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantCode {
				t.Fatalf("expected status %d, got %d", tc.wantCode, rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %s", tc.wantErr, rr.Body.String())
			}
		})
	}
}

// TestCreateRunHandlerModNotFound verifies that non-existent mod_id is rejected with 404.
func TestCreateRunHandlerModNotFound(t *testing.T) {
	st := &mockStore{
		createRunErr: &pgconn.PgError{Code: "23503"}, // foreign_key_violation
	}
	handler := createRunHandler(st)

	reqBody := map[string]interface{}{
		"mod_id":     uuid.New().String(),
		"base_ref":   "main",
		"target_ref": "feature",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "mod not found") {
		t.Errorf("expected error about mod not found, got: %s", rr.Body.String())
	}
}

// TestCreateRunHandlerMalformedJSON verifies that malformed JSON is rejected.
func TestCreateRunHandlerMalformedJSON(t *testing.T) {
	st := &mockStore{}
	handler := createRunHandler(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", strings.NewReader("{invalid json"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Errorf("expected error about invalid request, got: %s", rr.Body.String())
	}
}

// TestGetRunHandlerSuccess verifies successful run retrieval.
func TestGetRunHandlerSuccess(t *testing.T) {
	runID := uuid.New()
	modID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	stats := json.RawMessage(`{"lines": 100}`)

	st := &mockStore{
		getRunResult: store.Run{
			ID:         pgtype.UUID{Bytes: runID, Valid: true},
			ModID:      pgtype.UUID{Bytes: modID, Valid: true},
			Status:     store.RunStatusRunning,
			Reason:     strPtr("test reason"),
			CreatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt:  pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: now.Add(2 * time.Minute), Valid: true},
			NodeID:     pgtype.UUID{Bytes: nodeID, Valid: true},
			BaseRef:    "main",
			TargetRef:  "feature",
			CommitSha:  strPtr("abc123"),
			Stats:      stats,
		},
	}

	handler := getRunHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+runID.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID         string          `json:"id"`
		ModID      string          `json:"mod_id"`
		Status     string          `json:"status"`
		Reason     *string         `json:"reason,omitempty"`
		CreatedAt  string          `json:"created_at"`
		StartedAt  *string         `json:"started_at,omitempty"`
		FinishedAt *string         `json:"finished_at,omitempty"`
		NodeID     *string         `json:"node_id,omitempty"`
		BaseRef    string          `json:"base_ref"`
		TargetRef  string          `json:"target_ref"`
		CommitSha  *string         `json:"commit_sha,omitempty"`
		Stats      json.RawMessage `json:"stats,omitempty"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != runID.String() {
		t.Errorf("expected id %s, got %s", runID.String(), resp.ID)
	}
	if resp.ModID != modID.String() {
		t.Errorf("expected mod_id %s, got %s", modID.String(), resp.ModID)
	}
	if resp.Status != string(store.RunStatusRunning) {
		t.Errorf("expected status running, got %s", resp.Status)
	}
	if resp.Reason == nil || *resp.Reason != "test reason" {
		t.Error("expected reason 'test reason'")
	}
	if resp.NodeID == nil || *resp.NodeID != nodeID.String() {
		t.Error("expected node_id")
	}
	if resp.StartedAt == nil {
		t.Error("expected started_at")
	}
	if resp.FinishedAt == nil {
		t.Error("expected finished_at")
	}
	// Verify stats by comparing as JSON (ignoring whitespace).
	var respStatsJSON, expectedStatsJSON map[string]interface{}
	if err := json.Unmarshal(resp.Stats, &respStatsJSON); err != nil {
		t.Errorf("failed to unmarshal response stats: %v", err)
	}
	if err := json.Unmarshal(stats, &expectedStatsJSON); err != nil {
		t.Errorf("failed to unmarshal expected stats: %v", err)
	}
	if respStatsJSON["lines"] != expectedStatsJSON["lines"] {
		t.Errorf("expected stats %s, got %s", stats, resp.Stats)
	}

	if !st.getRunCalled {
		t.Error("expected GetRun to be called")
	}
}

// TestGetRunHandlerMissingID verifies that missing id query param is rejected.
func TestGetRunHandlerMissingID(t *testing.T) {
	st := &mockStore{}
	handler := getRunHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "id query parameter is required") {
		t.Errorf("expected error about missing id, got: %s", rr.Body.String())
	}
}

// TestGetRunHandlerInvalidID verifies that invalid id query param is rejected.
func TestGetRunHandlerInvalidID(t *testing.T) {
	st := &mockStore{}
	handler := getRunHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id=not-a-uuid", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid id") {
		t.Errorf("expected error about invalid id, got: %s", rr.Body.String())
	}
}

// TestGetRunHandlerNotFound verifies that non-existent run is rejected with 404.
func TestGetRunHandlerNotFound(t *testing.T) {
	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}
	handler := getRunHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+uuid.New().String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "run not found") {
		t.Errorf("expected error about run not found, got: %s", rr.Body.String())
	}
}

// TestGetRunTimingHandlerSuccess verifies successful run timing retrieval.
func TestGetRunTimingHandlerSuccess(t *testing.T) {
	runID := uuid.New()

	st := &mockStore{
		getRunTimingResult: store.RunsTiming{
			ID:      pgtype.UUID{Bytes: runID, Valid: true},
			QueueMs: 5000,
			RunMs:   120000,
		},
	}

	handler := getRunHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+runID.String()+"&view=timing", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID      string `json:"id"`
		QueueMs int64  `json:"queue_ms"`
		RunMs   int64  `json:"run_ms"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != runID.String() {
		t.Errorf("expected id %s, got %s", runID.String(), resp.ID)
	}
	if resp.QueueMs != 5000 {
		t.Errorf("expected queue_ms 5000, got %d", resp.QueueMs)
	}
	if resp.RunMs != 120000 {
		t.Errorf("expected run_ms 120000, got %d", resp.RunMs)
	}

	if !st.getRunTimingCalled {
		t.Error("expected GetRunTiming to be called")
	}
}

// TestGetRunTimingHandlerNotFound verifies that non-existent run timing is rejected with 404.
func TestGetRunTimingHandlerNotFound(t *testing.T) {
	st := &mockStore{
		getRunTimingErr: pgx.ErrNoRows,
	}
	handler := getRunHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+uuid.New().String()+"&view=timing", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "run not found") {
		t.Errorf("expected error about run not found, got: %s", rr.Body.String())
	}
}

// TestListRunTimingsHandlerSuccess verifies successful run timings listing.
func TestListRunTimingsHandlerSuccess(t *testing.T) {
	run1ID := uuid.New()
	run2ID := uuid.New()

	st := &mockStore{
		listRunsTimingsResult: []store.RunsTiming{
			{
				ID:      pgtype.UUID{Bytes: run1ID, Valid: true},
				QueueMs: 5000,
				RunMs:   120000,
			},
			{
				ID:      pgtype.UUID{Bytes: run2ID, Valid: true},
				QueueMs: 3000,
				RunMs:   90000,
			},
		},
	}

	handler := getRunHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?view=timing", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Timings []struct {
			ID      string `json:"id"`
			QueueMs int64  `json:"queue_ms"`
			RunMs   int64  `json:"run_ms"`
		} `json:"timings"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Timings) != 2 {
		t.Fatalf("expected 2 timings, got %d", len(resp.Timings))
	}

	if !st.listRunsTimingsCalled {
		t.Error("expected ListRunsTimings to be called")
	}

	// Verify default pagination params.
	if st.listRunsTimingsParams.Limit != 100 {
		t.Errorf("expected default limit 100, got %d", st.listRunsTimingsParams.Limit)
	}
	if st.listRunsTimingsParams.Offset != 0 {
		t.Errorf("expected default offset 0, got %d", st.listRunsTimingsParams.Offset)
	}
}

// TestListRunTimingsHandlerPagination verifies pagination parameters.
func TestListRunTimingsHandlerPagination(t *testing.T) {
	st := &mockStore{
		listRunsTimingsResult: []store.RunsTiming{},
	}

	handler := getRunHandler(st)

	cases := []struct {
		name        string
		query       string
		wantLimit   int32
		wantOffset  int32
		wantCode    int
		wantErrText string
	}{
		{
			name:       "custom limit and offset",
			query:      "view=timing&limit=50&offset=10",
			wantLimit:  50,
			wantOffset: 10,
			wantCode:   http.StatusOK,
		},
		{
			name:       "limit exceeds max",
			query:      "view=timing&limit=500",
			wantLimit:  200, // maxLimit
			wantOffset: 0,
			wantCode:   http.StatusOK,
		},
		{
			name:        "invalid limit",
			query:       "view=timing&limit=abc",
			wantCode:    http.StatusBadRequest,
			wantErrText: "invalid limit",
		},
		{
			name:        "invalid offset",
			query:       "view=timing&offset=-1",
			wantCode:    http.StatusBadRequest,
			wantErrText: "invalid offset",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock state.
			st.listRunsTimingsCalled = false
			st.listRunsTimingsParams = store.ListRunsTimingsParams{}

			req := httptest.NewRequest(http.MethodGet, "/v1/runs?"+tc.query, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantCode {
				t.Fatalf("expected status %d, got %d: %s", tc.wantCode, rr.Code, rr.Body.String())
			}

			if tc.wantCode == http.StatusOK {
				if st.listRunsTimingsParams.Limit != tc.wantLimit {
					t.Errorf("expected limit %d, got %d", tc.wantLimit, st.listRunsTimingsParams.Limit)
				}
				if st.listRunsTimingsParams.Offset != tc.wantOffset {
					t.Errorf("expected offset %d, got %d", tc.wantOffset, st.listRunsTimingsParams.Offset)
				}
			} else if tc.wantErrText != "" {
				if !strings.Contains(rr.Body.String(), tc.wantErrText) {
					t.Errorf("expected error containing %q, got: %s", tc.wantErrText, rr.Body.String())
				}
			}
		})
	}
}

// TestDeleteRunHandlerSuccess verifies successful run deletion.
func TestDeleteRunHandlerSuccess(t *testing.T) {
	runID := uuid.New()
	modID := uuid.New()
	now := time.Now()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			ModID:     pgtype.UUID{Bytes: modID, Valid: true},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := deleteRunHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if !st.getRunCalled {
		t.Error("expected GetRun to be called to verify existence")
	}
	if !st.deleteRunCalled {
		t.Error("expected DeleteRun to be called")
	}
}

// TestDeleteRunHandlerMissingID verifies that missing id path param is rejected.
func TestDeleteRunHandlerMissingID(t *testing.T) {
	st := &mockStore{}
	handler := deleteRunHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "id path parameter is required") {
		t.Errorf("expected error about missing id, got: %s", rr.Body.String())
	}
}

// TestDeleteRunHandlerInvalidID verifies that invalid id path param is rejected.
func TestDeleteRunHandlerInvalidID(t *testing.T) {
	st := &mockStore{}
	handler := deleteRunHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/not-a-uuid", nil)
	req.SetPathValue("id", "not-a-uuid")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid id") {
		t.Errorf("expected error about invalid id, got: %s", rr.Body.String())
	}
}

// TestDeleteRunHandlerNotFound verifies that non-existent run is rejected with 404.
func TestDeleteRunHandlerNotFound(t *testing.T) {
	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}
	handler := deleteRunHandler(st)

	runID := uuid.New().String()
	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/"+runID, nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "run not found") {
		t.Errorf("expected error about run not found, got: %s", rr.Body.String())
	}
}

// TestDeleteRunHandlerDatabaseError verifies that database errors are handled.
func TestDeleteRunHandlerDatabaseError(t *testing.T) {
	runID := uuid.New()
	modID := uuid.New()
	now := time.Now()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			ModID:     pgtype.UUID{Bytes: modID, Valid: true},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		deleteRunErr: errors.New("database connection failed"),
	}

	handler := deleteRunHandler(st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "failed to delete run") {
		t.Errorf("expected error about deletion failure, got: %s", rr.Body.String())
	}
}

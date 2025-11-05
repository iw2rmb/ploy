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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// newTestEventsService creates a minimal events service for testing.
func newTestEventsService() *events.Service {
	svc, _ := events.New(events.Options{
		BufferSize:  10,
		HistorySize: 100,
	})
	return svc
}

// TestSubmitTicketHandlerSuccess verifies successful ticket submission.
func TestSubmitTicketHandlerSuccess(t *testing.T) {
	runID := uuid.New()
	now := time.Now()

	st := &mockStore{
		createRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			RepoUrl:   "https://github.com/user/repo.git",
			Spec:      []byte("{}"),
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := submitTicketHandler(st, nil)

	reqBody := map[string]interface{}{
		"repo_url":   "https://github.com/user/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		TicketID  string `json:"ticket_id"`
		Status    string `json:"status"`
		RepoURL   string `json:"repo_url"`
		BaseRef   string `json:"base_ref"`
		TargetRef string `json:"target_ref"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.TicketID != runID.String() {
		t.Errorf("expected ticket_id %s, got %s", runID.String(), resp.TicketID)
	}
	if resp.Status != "queued" {
		t.Errorf("expected status queued, got %s", resp.Status)
	}
	if resp.RepoURL != "https://github.com/user/repo.git" {
		t.Errorf("expected repo_url https://github.com/user/repo.git, got %s", resp.RepoURL)
	}
	if resp.BaseRef != "main" {
		t.Errorf("expected base_ref main, got %s", resp.BaseRef)
	}
	if resp.TargetRef != "feature" {
		t.Errorf("expected target_ref feature, got %s", resp.TargetRef)
	}

	if !st.createRunCalled {
		t.Error("expected CreateRun to be called")
	}
}

// TestSubmitTicketHandlerMissingFields verifies validation of required fields.
func TestSubmitTicketHandlerMissingFields(t *testing.T) {
	st := &mockStore{}
	handler := submitTicketHandler(st, nil)

	cases := []struct {
		name string
		body map[string]interface{}
		err  string
	}{
		{"empty repo_url", map[string]interface{}{"repo_url": "", "base_ref": "main", "target_ref": "feature"}, "repo_url field is required"},
		{"whitespace repo_url", map[string]interface{}{"repo_url": "   ", "base_ref": "main", "target_ref": "feature"}, "repo_url field is required"},
		{"no repo_url", map[string]interface{}{"base_ref": "main", "target_ref": "feature"}, "repo_url field is required"},
		{"empty base_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "", "target_ref": "feature"}, "base_ref field is required"},
		{"whitespace base_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "   ", "target_ref": "feature"}, "base_ref field is required"},
		{"no base_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "target_ref": "feature"}, "base_ref field is required"},
		{"empty target_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "main", "target_ref": ""}, "target_ref field is required"},
		{"whitespace target_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "main", "target_ref": "   "}, "target_ref field is required"},
		{"no target_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "main"}, "target_ref field is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tc.err) {
				t.Errorf("expected error %q, got: %s", tc.err, rr.Body.String())
			}
		})
	}
}

// TestSubmitTicketHandlerInvalidJSON verifies rejection of malformed JSON.
func TestSubmitTicketHandlerInvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := submitTicketHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods", strings.NewReader("{invalid json"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Errorf("expected 'invalid request' error, got: %s", rr.Body.String())
	}
}

// TestSubmitTicketHandlerWithOptionalFields verifies optional fields are handled correctly.
func TestSubmitTicketHandlerWithOptionalFields(t *testing.T) {
	runID := uuid.New()
	now := time.Now()
	commitSha := "abc1234567890"
	createdBy := "user@example.com"
	customSpec := json.RawMessage(`{"key": "value"}`)

	st := &mockStore{
		createRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			RepoUrl:   "https://github.com/user/repo.git",
			Spec:      customSpec,
			CreatedBy: &createdBy,
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CommitSha: &commitSha,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := submitTicketHandler(st, nil)

	reqBody := map[string]interface{}{
		"repo_url":   "https://github.com/user/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
		"commit_sha": commitSha,
		"spec":       map[string]string{"key": "value"},
		"created_by": createdBy,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify run was created with custom spec (compare as JSON, not string).
	var expectedSpec, actualSpec map[string]interface{}
	if err := json.Unmarshal(customSpec, &expectedSpec); err != nil {
		t.Fatalf("failed to unmarshal expected spec: %v", err)
	}
	if err := json.Unmarshal(st.createRunParams.Spec, &actualSpec); err != nil {
		t.Fatalf("failed to unmarshal actual spec: %v", err)
	}
	if len(expectedSpec) != len(actualSpec) || expectedSpec["key"] != actualSpec["key"] {
		t.Errorf("expected spec %s, got %s", string(customSpec), string(st.createRunParams.Spec))
	}
	if st.createRunParams.CreatedBy == nil || *st.createRunParams.CreatedBy != createdBy {
		t.Error("expected created_by to be passed to CreateRun")
	}

	// Verify run was created with commit_sha.
	if st.createRunParams.CommitSha == nil || *st.createRunParams.CommitSha != commitSha {
		t.Error("expected commit_sha to be passed to CreateRun")
	}
}

// TestGetTicketStatusHandlerSuccess verifies successful retrieval of ticket status.
func TestGetTicketStatusHandlerSuccess(t *testing.T) {
	ticketID := uuid.New()
	now := time.Now()

	nodeID := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: ticketID, Valid: true},
			RepoUrl:   "https://github.com/user/repo.git",
			Status:    store.RunStatusRunning,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now.Add(5 * time.Second), Valid: true},
			NodeID:    pgtype.UUID{Bytes: nodeID, Valid: true},
		},
	}

	handler := getTicketStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+ticketID.String(), nil)
	req.SetPathValue("id", ticketID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp modsapi.TicketStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Ticket.TicketID != ticketID.String() {
		t.Errorf("expected ticket_id %s, got %s", ticketID.String(), resp.Ticket.TicketID)
	}
	if resp.Ticket.State != modsapi.TicketStateRunning {
		t.Errorf("expected status running, got %s", resp.Ticket.State)
	}
	if resp.Ticket.Repository != "https://github.com/user/repo.git" {
		t.Errorf("expected repo_url https://github.com/user/repo.git, got %s", resp.Ticket.Repository)
	}
	if resp.Ticket.Metadata["repo_base_ref"] != "main" {
		t.Errorf("expected base_ref main, got %s", resp.Ticket.Metadata["repo_base_ref"])
	}
	if resp.Ticket.Metadata["repo_target_ref"] != "feature" {
		t.Errorf("expected target_ref feature, got %s", resp.Ticket.Metadata["repo_target_ref"])
	}
	if got := resp.Ticket.Metadata["node_id"]; got != nodeID.String() {
		t.Errorf("expected node_id %s, got %s", nodeID.String(), got)
	}

	if !st.getRunCalled {
		t.Error("expected GetRun to be called")
	}
}

// TestGetTicketStatusHandlerNotFound verifies 404 when ticket doesn't exist.
func TestGetTicketStatusHandlerNotFound(t *testing.T) {
	ticketID := uuid.New()

	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}

	handler := getTicketStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+ticketID.String(), nil)
	req.SetPathValue("id", ticketID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ticket not found") {
		t.Errorf("expected 'ticket not found' error, got: %s", rr.Body.String())
	}

	if !st.getRunCalled {
		t.Error("expected GetRun to be called")
	}
}

// TestGetTicketStatusHandlerInvalidUUID verifies 400 when ticket ID is invalid.
func TestGetTicketStatusHandlerInvalidUUID(t *testing.T) {
	st := &mockStore{}
	handler := getTicketStatusHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/not-a-uuid", nil)
	req.SetPathValue("id", "not-a-uuid")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid ticket id") {
		t.Errorf("expected 'invalid ticket id' error, got: %s", rr.Body.String())
	}

	if st.getRunCalled {
		t.Error("expected GetRun NOT to be called for invalid UUID")
	}
}

// TestGetTicketStatusHandlerWithOptionalFields verifies optional fields are serialized correctly.
func TestGetTicketStatusHandlerWithOptionalFields(t *testing.T) {
	ticketID := uuid.New()
	now := time.Now()
	commitSha := "abc1234567890"
	reason := "run failed due to timeout"

	st := &mockStore{
		getRunResult: store.Run{
			ID:         pgtype.UUID{Bytes: ticketID, Valid: true},
			RepoUrl:    "https://github.com/user/repo.git",
			Status:     store.RunStatusFailed,
			Reason:     &reason,
			BaseRef:    "main",
			TargetRef:  "feature",
			CommitSha:  &commitSha,
			CreatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt:  pgtype.Timestamptz{Time: now.Add(5 * time.Second), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: now.Add(10 * time.Second), Valid: true},
		},
	}

	handler := getTicketStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+ticketID.String(), nil)
	req.SetPathValue("id", ticketID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp modsapi.TicketStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Reason should be surfaced under metadata.reason
	if resp.Ticket.Metadata["reason"] != reason {
		t.Errorf("expected reason %q, got %q", reason, resp.Ticket.Metadata["reason"])
	}
	if resp.Ticket.Repository != "https://github.com/user/repo.git" {
		t.Errorf("expected repo_url https://github.com/user/repo.git, got %s", resp.Ticket.Repository)
	}
	// FinishedAt not exposed directly; rely on state only.
}

// TestSubmitTicketHandlerPublishesEvent verifies that submitting a ticket publishes a queued event.
func TestSubmitTicketHandlerPublishesEvent(t *testing.T) {
	runID := uuid.New()
	now := time.Now()

	st := &mockStore{
		createRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			RepoUrl:   "https://github.com/user/repo.git",
			Spec:      []byte("{}"),
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	eventsService := newTestEventsService()
	handler := submitTicketHandler(st, eventsService)

	reqBody := map[string]interface{}{
		"repo_url":   "https://github.com/user/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify a ticket event was published to the hub by checking the snapshot.
	snapshot := eventsService.Hub().Snapshot(runID.String())
	if len(snapshot) == 0 {
		t.Fatal("expected at least one ticket event to be published")
	}

	// Verify the event type is "ticket".
	foundTicketEvent := false
	for _, evt := range snapshot {
		if evt.Type == "ticket" {
			foundTicketEvent = true
			// Verify the event contains ticket state information.
			if !strings.Contains(string(evt.Data), "queued") {
				t.Errorf("expected ticket event data to contain 'queued', got: %s", string(evt.Data))
			}
			break
		}
	}
	if !foundTicketEvent {
		t.Error("expected to find a 'ticket' event in the snapshot")
	}
}

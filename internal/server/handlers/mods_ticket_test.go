package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
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
			ID:        runID.String(),
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
		TicketID  string `json:"run_id"`
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
// Domain types now validate at JSON unmarshal time, rejecting empty/invalid values.
// Note: target_ref is optional in the handler; omitted target_ref is allowed.
func TestSubmitTicketHandlerMissingFields(t *testing.T) {
	st := &mockStore{}
	handler := submitTicketHandler(st, nil)

	cases := []struct {
		name string
		body map[string]interface{}
		err  string // Expected error substring (domain validation errors)
	}{
		{"empty repo_url", map[string]interface{}{"repo_url": "", "base_ref": "main", "target_ref": "feature"}, "empty"},
		{"whitespace repo_url", map[string]interface{}{"repo_url": "   ", "base_ref": "main", "target_ref": "feature"}, "empty"},
		{"no repo_url", map[string]interface{}{"base_ref": "main", "target_ref": "feature"}, "empty"},
		{"empty base_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "", "target_ref": "feature"}, "empty"},
		{"whitespace base_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "   ", "target_ref": "feature"}, "empty"},
		{"no base_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "target_ref": "feature"}, "empty"},
		{"empty target_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "main", "target_ref": ""}, "empty"},
		{"whitespace target_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "main", "target_ref": "   "}, "empty"},
		// Note: "no target_ref" removed - target_ref is optional in the handler.
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

// TestSubmitTicketHandlerInvalidRepoURL verifies domain type validation for repo URLs.
func TestSubmitTicketHandlerInvalidRepoURL(t *testing.T) {
	st := &mockStore{}
	handler := submitTicketHandler(st, nil)

	cases := []struct {
		name    string
		repoURL string
		errMsg  string
	}{
		{"http scheme", "http://github.com/user/repo.git", "invalid repo url"},
		{"git scheme", "git://github.com/user/repo.git", "invalid repo url"},
		{"no scheme", "github.com/user/repo.git", "invalid repo url"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]interface{}{
				"repo_url":   tc.repoURL,
				"base_ref":   "main",
				"target_ref": "feature",
			}
			bodyBytes, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(bodyBytes))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tc.errMsg) {
				t.Errorf("expected error %q, got: %s", tc.errMsg, rr.Body.String())
			}
		})
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
			ID:        runID.String(),
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
	nodeIDStr := nodeID.String()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        ticketID.String(),
			RepoUrl:   "https://github.com/user/repo.git",
			Status:    store.RunStatusRunning,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now.Add(5 * time.Second), Valid: true},
			NodeID:    &nodeIDStr,
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

	var resp modsapi.RunStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if string(resp.Ticket.TicketID) != ticketID.String() {
		t.Errorf("expected ticket_id %s, got %s", ticketID.String(), string(resp.Ticket.TicketID))
	}
	if resp.Ticket.State != modsapi.RunStateRunning {
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
	if !strings.Contains(rr.Body.String(), "not found") {
		t.Errorf("expected 'not found' error, got: %s", rr.Body.String())
	}

	if !st.getRunCalled {
		t.Error("expected GetRun to be called")
	}
}

// TestGetTicketStatusHandlerEmptyID verifies 400 when ticket ID is empty.
// Run IDs are now KSUID strings; only empty/whitespace IDs are rejected.
// Note: "not-a-uuid" is now a valid KSUID string ID, so this test only checks empty ID.
func TestGetTicketStatusHandlerEmptyID(t *testing.T) {
	st := &mockStore{}
	handler := getTicketStatusHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}

	if st.getRunCalled {
		t.Error("expected GetRun NOT to be called for empty ID")
	}
}

// TestGetTicketStatusHandlerWithOptionalFields verifies optional fields are serialized correctly.
func TestGetTicketStatusHandlerWithOptionalFields(t *testing.T) {
	ticketID := uuid.New()
	now := time.Now()
	commitSha := "abc1234567890"
	// Include MR URL under runs.stats.metadata to verify surfacing in response metadata.
	stats := []byte(`{"metadata":{"mr_url":"https://gitlab.com/org/repo/-/merge_requests/99"}}`)

	st := &mockStore{
		getRunResult: store.Run{
			ID:         ticketID.String(),
			RepoUrl:    "https://github.com/user/repo.git",
			Status:     store.RunStatusFailed,
			BaseRef:    "main",
			TargetRef:  "feature",
			CommitSha:  &commitSha,
			CreatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt:  pgtype.Timestamptz{Time: now.Add(5 * time.Second), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: now.Add(10 * time.Second), Valid: true},
			Stats:      stats,
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

	var resp modsapi.RunStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Ticket.Repository != "https://github.com/user/repo.git" {
		t.Errorf("expected repo_url https://github.com/user/repo.git, got %s", resp.Ticket.Repository)
	}
	// MR URL should be propagated from stats.metadata.mr_url
	if resp.Ticket.Metadata["mr_url"] != "https://gitlab.com/org/repo/-/merge_requests/99" {
		t.Errorf("expected mr_url to be present, got %q", resp.Ticket.Metadata["mr_url"])
	}
	// FinishedAt not exposed directly; rely on state only.
}

// TestSubmitTicketHandlerPublishesEvent verifies that submitting a ticket publishes a queued event.
func TestSubmitTicketHandlerPublishesEvent(t *testing.T) {
	runID := uuid.New()
	now := time.Now()

	st := &mockStore{
		createRunResult: store.Run{
			ID:        runID.String(),
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

	// Verify the event type is "run".
	foundTicketEvent := false
	for _, evt := range snapshot {
		if evt.Type == "run" {
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

// TestSubmitTicketHandlerMultiStepCreatesMultipleStages verifies that submitting
// a multi-step spec (with mods[] array) creates one job per mod.
func TestSubmitTicketHandlerMultiStepCreatesMultipleStages(t *testing.T) {
	runID := uuid.New()
	now := time.Now()

	st := &mockStore{
		createRunResult: store.Run{
			ID:        runID.String(),
			RepoUrl:   "https://github.com/user/repo.git",
			Spec:      []byte(`{"mods":[{"image":"img1:latest"},{"image":"img2:latest"},{"image":"img3:latest"}]}`),
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
		"spec": map[string]interface{}{
			"mods": []map[string]string{
				{"image": "img1:latest"},
				{"image": "img2:latest"},
				{"image": "img3:latest"},
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify 5 jobs were created: pre-gate + 3 mods + post-gate.
	if st.createJobCallCount != 5 {
		t.Errorf("expected 5 CreateJob calls (pre-gate + 3 mods + post-gate), got %d", st.createJobCallCount)
	}

	// Verify job names: pre-gate, mod-0, mod-1, mod-2, post-gate.
	expectedJobNames := []string{"pre-gate", "mod-0", "mod-1", "mod-2", "post-gate"}
	if len(st.createJobParams) != 5 {
		t.Fatalf("expected 5 job params, got %d", len(st.createJobParams))
	}
	for i, expected := range expectedJobNames {
		if st.createJobParams[i].Name != expected {
			t.Errorf("expected job %d name %q, got %q", i, expected, st.createJobParams[i].Name)
		}
	}

	// Verify mod jobs (index 1-3) have correct mod_image column.
	for i := 1; i <= 3; i++ {
		params := st.createJobParams[i]
		expectedImage := fmt.Sprintf("img%d:latest", i)
		if params.ModImage != expectedImage {
			t.Errorf("expected mod job %d mod_image %q, got %q", i, expectedImage, params.ModImage)
		}
	}
}

// TestSubmitTicketHandlerSingleStepCreatesThreeJobs verifies that submitting
// a single-step spec creates the standard 3-job pipeline: pre-gate, mod-0, post-gate.
func TestSubmitTicketHandlerSingleStepCreatesThreeJobs(t *testing.T) {
	cases := []struct {
		name      string
		spec      map[string]interface{}
		wantNames []string
	}{
		{
			name:      "mod section",
			spec:      map[string]interface{}{"mod": map[string]string{"image": "single:latest"}},
			wantNames: []string{"pre-gate", "mod-0", "post-gate"},
		},
		{
			name:      "legacy top-level",
			spec:      map[string]interface{}{"image": "legacy:latest"},
			wantNames: []string{"pre-gate", "mod-0", "post-gate"},
		},
		{
			name:      "empty spec",
			spec:      map[string]interface{}{},
			wantNames: []string{"pre-gate", "mod-0", "post-gate"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := uuid.New()
			now := time.Now()

			specBytes, _ := json.Marshal(tc.spec)
			st := &mockStore{
				createRunResult: store.Run{
					ID:        runID.String(),
					RepoUrl:   "https://github.com/user/repo.git",
					Spec:      specBytes,
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
				"spec":       tc.spec,
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
			}

			// Verify 3 jobs were created: pre-gate, mod-0, post-gate.
			if st.createJobCallCount != 3 {
				t.Errorf("expected 3 CreateJob calls (pre-gate + mod-0 + post-gate), got %d", st.createJobCallCount)
			}

			// Verify job names match expected.
			if len(st.createJobParams) != 3 {
				t.Fatalf("expected 3 job params, got %d", len(st.createJobParams))
			}
			for i, wantName := range tc.wantNames {
				if st.createJobParams[i].Name != wantName {
					t.Errorf("expected job %d name %q, got %q", i, wantName, st.createJobParams[i].Name)
				}
			}
		})
	}
}

// TestSubmitTicketHandlerMultiStepNoRunSteps verifies that submitting
// a multi-step spec (with mods[] array) creates jobs but NOT run_steps.
// Run steps have been replaced by jobs in the new architecture.
func TestSubmitTicketHandlerMultiStepNoRunSteps(t *testing.T) {
	runID := uuid.New()
	now := time.Now()

	st := &mockStore{
		createRunResult: store.Run{
			ID:        runID.String(),
			RepoUrl:   "https://github.com/user/repo.git",
			Spec:      []byte(`{"mods":[{"image":"img1:latest"},{"image":"img2:latest"},{"image":"img3:latest"}]}`),
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
		"spec": map[string]interface{}{
			"mods": []map[string]string{
				{"image": "img1:latest"},
				{"image": "img2:latest"},
				{"image": "img3:latest"},
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify 5 jobs were created: pre-gate + 3 mods + post-gate.
	if st.createJobCallCount != 5 {
		t.Errorf("expected 5 CreateJob calls, got %d", st.createJobCallCount)
	}
}

// TestSubmitTicketHandlerSingleStep verifies that submitting
// a single-step spec (with mod section or legacy top-level) creates a single job.
func TestSubmitTicketHandlerSingleStep(t *testing.T) {
	cases := []struct {
		name string
		spec map[string]interface{}
	}{
		{
			name: "mod section",
			spec: map[string]interface{}{"mod": map[string]string{"image": "single:latest"}},
		},
		{
			name: "legacy top-level",
			spec: map[string]interface{}{"image": "legacy:latest"},
		},
		{
			name: "empty spec",
			spec: map[string]interface{}{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := uuid.New()
			now := time.Now()

			specBytes, _ := json.Marshal(tc.spec)
			st := &mockStore{
				createRunResult: store.Run{
					ID:        runID.String(),
					RepoUrl:   "https://github.com/user/repo.git",
					Spec:      specBytes,
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
				"spec":       tc.spec,
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
			}

			// Verify job(s) were created for this single-step run.
			if st.createJobCallCount == 0 {
				t.Errorf("expected CreateJob to be called, but it wasn't")
			}
		})
	}
}

// TestGetTicketStatusHandlerExposesStepIndex verifies that GET /v1/mods/{id}
// exposes step_index for each job based on the job's StepIndex field.
func TestGetTicketStatusHandlerExposesStepIndex(t *testing.T) {
	ticketID := uuid.New()
	now := time.Now()

	// Create mock jobs with step_index field set.
	// Note: StepIndex is read from the Job struct directly, not from metadata.
	job0 := store.Job{
		ID:        types.NewJobID().String(),
		RunID:     ticketID.String(),
		Name:      "mod-0",
		Status:    store.JobStatusCreated,
		StepIndex: 2000, // First mod job
		Meta:      []byte(`{"mod_type":"mod","mod_image":"img1:latest"}`),
	}
	job1 := store.Job{
		ID:        types.NewJobID().String(),
		RunID:     ticketID.String(),
		Name:      "mod-1",
		Status:    store.JobStatusCreated,
		StepIndex: 3000, // Second mod job
		Meta:      []byte(`{"mod_type":"mod","mod_image":"img2:latest"}`),
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:        ticketID.String(),
			RepoUrl:   "https://github.com/user/repo.git",
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		listJobsByRunResult: []store.Job{job0, job1},
	}

	handler := getTicketStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+ticketID.String(), nil)
	req.SetPathValue("id", ticketID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp modsapi.RunStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify both jobs are present with correct step_index.
	if len(resp.Ticket.Stages) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(resp.Ticket.Stages))
	}

	job0ID := job0.ID
	job1ID := job1.ID

	if _, ok := resp.Ticket.Stages[job0ID]; !ok {
		t.Errorf("expected job %s to be present", job0ID)
	}
	if resp.Ticket.Stages[job0ID].StepIndex != 2000 {
		t.Errorf("expected job 0 step_index 2000, got %d", resp.Ticket.Stages[job0ID].StepIndex)
	}

	if _, ok := resp.Ticket.Stages[job1ID]; !ok {
		t.Errorf("expected job %s to be present", job1ID)
	}
	if resp.Ticket.Stages[job1ID].StepIndex != 3000 {
		t.Errorf("expected job 1 step_index 3000, got %d", resp.Ticket.Stages[job1ID].StepIndex)
	}
}

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

// TestSubmitRunHandlerSuccess verifies successful run submission.
//
// Canonical contract verification:
//   - HTTP 201 Created (not 202 or other legacy codes)
//   - Response body is RunSummary directly (no wrapper types)
//   - run_id is a KSUID string
//   - state field uses canonical enum values (pending, running, etc.)
func TestSubmitRunHandlerSuccess(t *testing.T) {
	runID := types.NewRunID()
	now := time.Now()

	execRun := store.Run{
		ID:        runID.String(),
		RepoUrl:   "https://github.com/user/repo.git",
		Spec:      []byte("{}"),
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoID := types.NewRunRepoID()
	repoPending := store.RunRepo{
		ID:        string(repoID),
		RunID:     types.RunID(runID.String()),
		RepoUrl:   "https://github.com/user/repo.git",
		BaseRef:   "main",
		TargetRef: "feature",
		Status:    store.RunRepoStatusPending,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoRunning := repoPending
	execID := execRun.ID
	repoRunning.Status = store.RunRepoStatusRunning
	repoRunning.ExecutionRunID = &execID

	st := &mockStore{
		createRunResult:                execRun,
		getRunResult:                   execRun,
		createRunRepoResult:            repoPending,
		listPendingRunReposByRunResult: []store.RunRepo{repoPending},
		getRunRepoResult:               repoRunning,
	}

	handler := submitRunHandler(st, nil)

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

	// Decode RunSummary directly — POST /v1/mods returns the canonical type.
	var resp modsapi.RunSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if string(resp.RunID) != runID.String() {
		t.Errorf("expected run_id %s, got %s", runID.String(), string(resp.RunID))
	}
	if resp.State != modsapi.RunStatePending {
		t.Errorf("expected state pending, got %s", resp.State)
	}
	if resp.Repository != "https://github.com/user/repo.git" {
		t.Errorf("expected repository https://github.com/user/repo.git, got %s", resp.Repository)
	}
	if resp.Metadata["repo_base_ref"] != "main" {
		t.Errorf("expected metadata[repo_base_ref] main, got %s", resp.Metadata["repo_base_ref"])
	}
	if resp.Metadata["repo_target_ref"] != "feature" {
		t.Errorf("expected metadata[repo_target_ref] feature, got %s", resp.Metadata["repo_target_ref"])
	}

	if !st.createRunCalled {
		t.Error("expected CreateRun to be called")
	}
}

// TestSubmitRunHandlerMissingFields verifies validation of required fields.
// Domain types now validate at JSON unmarshal time, rejecting empty/invalid values.
// Note: target_ref is optional in the handler; omitted target_ref is allowed.
func TestSubmitRunHandlerMissingFields(t *testing.T) {
	st := &mockStore{}
	handler := submitRunHandler(st, nil)

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

// TestSubmitRunHandlerInvalidJSON verifies rejection of malformed JSON.
func TestSubmitRunHandlerInvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := submitRunHandler(st, nil)

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

// TestSubmitRunHandlerInvalidRepoURL verifies domain type validation for repo URLs.
func TestSubmitRunHandlerInvalidRepoURL(t *testing.T) {
	st := &mockStore{}
	handler := submitRunHandler(st, nil)

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

// TestSubmitRunHandlerWithOptionalFields verifies optional fields are handled correctly.
func TestSubmitRunHandlerWithOptionalFields(t *testing.T) {
	runID := types.NewRunID()
	now := time.Now()
	commitSha := "abc1234567890"
	createdBy := "user@example.com"
	customSpec := json.RawMessage(`{"key": "value"}`)

	execRun := store.Run{
		ID:        runID.String(),
		RepoUrl:   "https://github.com/user/repo.git",
		Spec:      customSpec,
		CreatedBy: &createdBy,
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
		CommitSha: &commitSha,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoID := types.NewRunRepoID()
	repoPending := store.RunRepo{
		ID:        string(repoID),
		RunID:     types.RunID(runID.String()),
		RepoUrl:   "https://github.com/user/repo.git",
		BaseRef:   "main",
		TargetRef: "feature",
		Status:    store.RunRepoStatusPending,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoRunning := repoPending
	execID := execRun.ID
	repoRunning.Status = store.RunRepoStatusRunning
	repoRunning.ExecutionRunID = &execID

	st := &mockStore{
		createRunResult:                execRun,
		getRunResult:                   execRun,
		createRunRepoResult:            repoPending,
		listPendingRunReposByRunResult: []store.RunRepo{repoPending},
		getRunRepoResult:               repoRunning,
	}

	handler := submitRunHandler(st, nil)

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

// TestGetRunStatusHandlerSuccess verifies successful retrieval of run status.
//
// Canonical contract verification:
//   - HTTP 200 OK
//   - Response body is RunSummary directly (no wrapper types)
//   - run_id field uses canonical JSON key (not legacy "id" or "ticket_id")
//   - stages map is keyed by job ID (KSUID string)
func TestGetRunStatusHandlerSuccess(t *testing.T) {
	runID := types.NewRunID()
	now := time.Now()

	nodeID := types.NewNodeKey()
	nodeIDStr := nodeID
	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID.String(),
			RepoUrl:   "https://github.com/user/repo.git",
			Status:    store.RunStatusRunning,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now.Add(5 * time.Second), Valid: true},
			NodeID:    &nodeIDStr,
		},
	}

	handler := getRunStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Decode RunSummary directly — the server returns the canonical type (no wrapper).
	var resp modsapi.RunSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify RunID field is correctly populated.
	if string(resp.RunID) != runID.String() {
		t.Errorf("expected run_id %s, got %s", runID.String(), string(resp.RunID))
	}
	if resp.State != modsapi.RunStateRunning {
		t.Errorf("expected status running, got %s", resp.State)
	}
	if resp.Repository != "https://github.com/user/repo.git" {
		t.Errorf("expected repo_url https://github.com/user/repo.git, got %s", resp.Repository)
	}
	if resp.Metadata["repo_base_ref"] != "main" {
		t.Errorf("expected base_ref main, got %s", resp.Metadata["repo_base_ref"])
	}
	if resp.Metadata["repo_target_ref"] != "feature" {
		t.Errorf("expected target_ref feature, got %s", resp.Metadata["repo_target_ref"])
	}
	if got := resp.Metadata["node_id"]; got != nodeID {
		t.Errorf("expected node_id %s, got %s", nodeID, got)
	}

	if !st.getRunCalled {
		t.Error("expected GetRun to be called")
	}
}

// TestGetRunStatusHandlerNotFound verifies 404 when run doesn't exist.
func TestGetRunStatusHandlerNotFound(t *testing.T) {
	runID := types.NewRunID()

	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}

	handler := getRunStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
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

// TestGetRunStatusHandlerEmptyID verifies 400 when run ID is empty.
// Run IDs are now KSUID strings; only empty/whitespace IDs are rejected.
// Note: "not-a-uuid" is now a valid KSUID string ID, so this test only checks empty ID.
func TestGetRunStatusHandlerEmptyID(t *testing.T) {
	st := &mockStore{}
	handler := getRunStatusHandler(st)

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

// TestGetRunStatusHandlerWithOptionalFields verifies optional fields are serialized correctly.
func TestGetRunStatusHandlerWithOptionalFields(t *testing.T) {
	runID := types.NewRunID()
	now := time.Now()
	commitSha := "abc1234567890"
	// Include MR URL under runs.stats.metadata to verify surfacing in response metadata.
	stats := []byte(`{"metadata":{"mr_url":"https://gitlab.com/org/repo/-/merge_requests/99"}}`)

	st := &mockStore{
		getRunResult: store.Run{
			ID:         runID.String(),
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

	handler := getRunStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Decode RunSummary directly — the server returns the canonical type (no wrapper).
	var resp modsapi.RunSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Repository != "https://github.com/user/repo.git" {
		t.Errorf("expected repo_url https://github.com/user/repo.git, got %s", resp.Repository)
	}
	// MR URL should be propagated from stats.metadata.mr_url
	if resp.Metadata["mr_url"] != "https://gitlab.com/org/repo/-/merge_requests/99" {
		t.Errorf("expected mr_url to be present, got %q", resp.Metadata["mr_url"])
	}
	// FinishedAt not exposed directly; rely on state only.
}

// TestSubmitRunHandlerPublishesEvent verifies that submitting a run publishes a queued event.
func TestSubmitRunHandlerPublishesEvent(t *testing.T) {
	runID := types.NewRunID()
	now := time.Now()

	execRun := store.Run{
		ID:        runID.String(),
		RepoUrl:   "https://github.com/user/repo.git",
		Spec:      []byte("{}"),
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoID := types.NewRunRepoID()
	repoPending := store.RunRepo{
		ID:        string(repoID),
		RunID:     types.RunID(runID.String()),
		RepoUrl:   "https://github.com/user/repo.git",
		BaseRef:   "main",
		TargetRef: "feature",
		Status:    store.RunRepoStatusPending,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoRunning := repoPending
	execID := execRun.ID
	repoRunning.Status = store.RunRepoStatusRunning
	repoRunning.ExecutionRunID = &execID

	st := &mockStore{
		createRunResult:                execRun,
		getRunResult:                   execRun,
		createRunRepoResult:            repoPending,
		listPendingRunReposByRunResult: []store.RunRepo{repoPending},
		getRunRepoResult:               repoRunning,
	}

	eventsService := newTestEventsService()
	handler := submitRunHandler(st, eventsService)

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

	// Verify a run event was published to the hub by checking the snapshot.
	snapshot := eventsService.Hub().Snapshot(runID.String())
	if len(snapshot) == 0 {
		t.Fatal("expected at least one run event to be published")
	}

	// Verify the event type is "run".
	foundRunEvent := false
	for _, evt := range snapshot {
		if evt.Type == "run" {
			foundRunEvent = true
			// Verify the event contains run state information.
			if !strings.Contains(string(evt.Data), "\"state\":\"pending\"") {
				t.Errorf("expected run event data to contain state \"pending\", got: %s", string(evt.Data))
			}
			break
		}
	}
	if !foundRunEvent {
		t.Error("expected to find a 'run' event in the snapshot")
	}
}

// TestSubmitRunHandlerMultiStepCreatesMultipleStages verifies that submitting
// a multi-step spec (with mods[] array) creates one job per mod.
func TestSubmitRunHandlerMultiStepCreatesMultipleStages(t *testing.T) {
	runID := types.NewRunID()
	now := time.Now()

	specBytes := []byte(`{"mods":[{"image":"img1:latest"},{"image":"img2:latest"},{"image":"img3:latest"}]}`)
	execRun := store.Run{
		ID:        runID.String(),
		RepoUrl:   "https://github.com/user/repo.git",
		Spec:      specBytes,
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoID := types.NewRunRepoID()
	repoPending := store.RunRepo{
		ID:        string(repoID),
		RunID:     types.RunID(runID.String()),
		RepoUrl:   "https://github.com/user/repo.git",
		BaseRef:   "main",
		TargetRef: "feature",
		Status:    store.RunRepoStatusPending,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoRunning := repoPending
	execID := execRun.ID
	repoRunning.Status = store.RunRepoStatusRunning
	repoRunning.ExecutionRunID = &execID

	st := &mockStore{
		createRunResult:                execRun,
		getRunResult:                   execRun,
		createRunRepoResult:            repoPending,
		listPendingRunReposByRunResult: []store.RunRepo{repoPending},
		getRunRepoResult:               repoRunning,
	}

	handler := submitRunHandler(st, nil)

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

// TestSubmitRunHandlerSingleStepCreatesThreeJobs verifies that submitting
// a single-step spec creates the standard 3-job pipeline: pre-gate, mod-0, post-gate.
func TestSubmitRunHandlerSingleStepCreatesThreeJobs(t *testing.T) {
	cases := []struct {
		name      string
		spec      map[string]interface{}
		wantNames []string
	}{
		{
			name:      "top-level image",
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
			runID := types.NewRunID()
			now := time.Now()

			specBytes, _ := json.Marshal(tc.spec)
			execRun := store.Run{
				ID:        runID.String(),
				RepoUrl:   "https://github.com/user/repo.git",
				Spec:      specBytes,
				Status:    store.RunStatusQueued,
				BaseRef:   "main",
				TargetRef: "feature",
				CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			}
			repoID := types.NewRunRepoID()
			repoPending := store.RunRepo{
				ID:        string(repoID),
				RunID:     types.RunID(runID.String()),
				RepoUrl:   "https://github.com/user/repo.git",
				BaseRef:   "main",
				TargetRef: "feature",
				Status:    store.RunRepoStatusPending,
				CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			}
			repoRunning := repoPending
			execID := execRun.ID
			repoRunning.Status = store.RunRepoStatusRunning
			repoRunning.ExecutionRunID = &execID

			st := &mockStore{
				createRunResult:                execRun,
				getRunResult:                   execRun,
				createRunRepoResult:            repoPending,
				listPendingRunReposByRunResult: []store.RunRepo{repoPending},
				getRunRepoResult:               repoRunning,
			}

			handler := submitRunHandler(st, nil)

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

func TestSubmitRunHandlerRejectsLegacyModSection(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := submitRunHandler(st, nil)

	reqBody := map[string]interface{}{
		"repo_url":   "https://github.com/user/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       map[string]any{"mod": map[string]any{"image": "single:latest"}},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestSubmitRunHandlerMultiStepNoRunSteps verifies that submitting
// a multi-step spec (with mods[] array) creates jobs but NOT run_steps.
// Run steps have been replaced by jobs in the new architecture.
func TestSubmitRunHandlerMultiStepNoRunSteps(t *testing.T) {
	runID := types.NewRunID()
	now := time.Now()

	specBytes := []byte(`{"mods":[{"image":"img1:latest"},{"image":"img2:latest"},{"image":"img3:latest"}]}`)
	execRun := store.Run{
		ID:        runID.String(),
		RepoUrl:   "https://github.com/user/repo.git",
		Spec:      specBytes,
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoID := types.NewRunRepoID()
	repoPending := store.RunRepo{
		ID:        string(repoID),
		RunID:     types.RunID(runID.String()),
		RepoUrl:   "https://github.com/user/repo.git",
		BaseRef:   "main",
		TargetRef: "feature",
		Status:    store.RunRepoStatusPending,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoRunning := repoPending
	execID := execRun.ID
	repoRunning.Status = store.RunRepoStatusRunning
	repoRunning.ExecutionRunID = &execID

	st := &mockStore{
		createRunResult:                execRun,
		getRunResult:                   execRun,
		createRunRepoResult:            repoPending,
		listPendingRunReposByRunResult: []store.RunRepo{repoPending},
		getRunRepoResult:               repoRunning,
	}

	handler := submitRunHandler(st, nil)

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

// TestSubmitRunHandlerSingleStep verifies that submitting
// a single-step spec (with mod section or legacy top-level) creates a single job.
func TestSubmitRunHandlerSingleStep(t *testing.T) {
	cases := []struct {
		name string
		spec map[string]interface{}
	}{
		{
			name: "top-level image",
			spec: map[string]interface{}{"image": "legacy:latest"},
		},
		{
			name: "empty spec",
			spec: map[string]interface{}{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := types.NewRunID()
			now := time.Now()

			specBytes, _ := json.Marshal(tc.spec)
			execRun := store.Run{
				ID:        runID.String(),
				RepoUrl:   "https://github.com/user/repo.git",
				Spec:      specBytes,
				Status:    store.RunStatusQueued,
				BaseRef:   "main",
				TargetRef: "feature",
				CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			}
			repoID := types.NewRunRepoID()
			repoPending := store.RunRepo{
				ID:        string(repoID),
				RunID:     types.RunID(runID.String()),
				RepoUrl:   "https://github.com/user/repo.git",
				BaseRef:   "main",
				TargetRef: "feature",
				Status:    store.RunRepoStatusPending,
				CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			}
			repoRunning := repoPending
			execID := execRun.ID
			repoRunning.Status = store.RunRepoStatusRunning
			repoRunning.ExecutionRunID = &execID

			st := &mockStore{
				createRunResult:                execRun,
				getRunResult:                   execRun,
				createRunRepoResult:            repoPending,
				listPendingRunReposByRunResult: []store.RunRepo{repoPending},
				getRunRepoResult:               repoRunning,
			}

			handler := submitRunHandler(st, nil)

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

// TestGetRunStatusHandlerExposesStepIndex verifies that GET /v1/runs/{id}/status
// exposes step_index for each job based on the job's StepIndex field.
func TestGetRunStatusHandlerExposesStepIndex(t *testing.T) {
	runID := types.NewRunID()
	now := time.Now()

	// Create mock jobs with step_index field set.
	// Note: StepIndex is read from the Job struct directly, not from metadata.
	job0 := store.Job{
		ID:        types.NewJobID().String(),
		RunID:     runID,
		Name:      "mod-0",
		Status:    store.JobStatusCreated,
		StepIndex: 2000, // First mod job
		Meta:      []byte(`{"mod_type":"mod","mod_image":"img1:latest"}`),
	}
	job1 := store.Job{
		ID:        types.NewJobID().String(),
		RunID:     runID,
		Name:      "mod-1",
		Status:    store.JobStatusCreated,
		StepIndex: 3000, // Second mod job
		Meta:      []byte(`{"mod_type":"mod","mod_image":"img2:latest"}`),
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID.String(),
			RepoUrl:   "https://github.com/user/repo.git",
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		listJobsByRunResult: []store.Job{job0, job1},
	}

	handler := getRunStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Decode RunSummary directly — the server returns the canonical type (no wrapper).
	var resp modsapi.RunSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify both jobs are present with correct step_index.
	if len(resp.Stages) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(resp.Stages))
	}

	job0ID := job0.ID
	job1ID := job1.ID

	if _, ok := resp.Stages[job0ID]; !ok {
		t.Errorf("expected job %s to be present", job0ID)
	}
	if resp.Stages[job0ID].StepIndex != 2000 {
		t.Errorf("expected job 0 step_index 2000, got %d", resp.Stages[job0ID].StepIndex)
	}

	if _, ok := resp.Stages[job1ID]; !ok {
		t.Errorf("expected job %s to be present", job1ID)
	}
	if resp.Stages[job1ID].StepIndex != 3000 {
		t.Errorf("expected job 1 step_index 3000, got %d", resp.Stages[job1ID].StepIndex)
	}
}

// TestSubmitRunHandlerCanonicalContract verifies that the submit handler returns
// only the canonical response shape (RunSummary directly, no legacy envelope).
//
// This test ensures the server aligns with the canonical CLI contract per ROADMAP.md:
//   - No "ticket" wrapper field
//   - No legacy field names (e.g., "ticket_id", "status" instead of "state")
//   - RunSummary is the JSON root object
func TestSubmitRunHandlerCanonicalContract(t *testing.T) {
	runID := types.NewRunID()
	now := time.Now()

	execRun := store.Run{
		ID:        runID.String(),
		RepoUrl:   "https://github.com/user/repo.git",
		Spec:      []byte("{}"),
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoID := types.NewRunRepoID()
	repoPending := store.RunRepo{
		ID:        string(repoID),
		RunID:     types.RunID(runID.String()),
		RepoUrl:   "https://github.com/user/repo.git",
		BaseRef:   "main",
		TargetRef: "feature",
		Status:    store.RunRepoStatusPending,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	repoRunning := repoPending
	execID := execRun.ID
	repoRunning.Status = store.RunRepoStatusRunning
	repoRunning.ExecutionRunID = &execID

	st := &mockStore{
		createRunResult:                execRun,
		getRunResult:                   execRun,
		createRunRepoResult:            repoPending,
		listPendingRunReposByRunResult: []store.RunRepo{repoPending},
		getRunRepoResult:               repoRunning,
	}

	handler := submitRunHandler(st, nil)

	reqBody := map[string]interface{}{
		"repo_url":   "https://github.com/user/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Canonical contract: HTTP 201 (not 202).
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Parse response as raw JSON to verify no legacy fields are present.
	var rawResp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&rawResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify canonical fields are present.
	if _, ok := rawResp["run_id"]; !ok {
		t.Error("expected 'run_id' field in response (canonical)")
	}
	if _, ok := rawResp["state"]; !ok {
		t.Error("expected 'state' field in response (canonical)")
	}
	if _, ok := rawResp["stages"]; !ok {
		t.Error("expected 'stages' field in response (canonical)")
	}

	// Verify legacy/envelope fields are NOT present.
	if _, ok := rawResp["ticket"]; ok {
		t.Error("unexpected 'ticket' wrapper field in response (legacy)")
	}
	if _, ok := rawResp["ticket_id"]; ok {
		t.Error("unexpected 'ticket_id' field in response (legacy)")
	}
	if _, ok := rawResp["status"]; ok {
		t.Error("unexpected 'status' field in response (legacy - use 'state')")
	}
}

// TestGetRunStatusHandlerCanonicalContract verifies that the status handler returns
// only the canonical response shape (RunSummary directly, no legacy envelope).
//
// This test ensures the server aligns with the canonical CLI contract per ROADMAP.md:
//   - No "ticket" wrapper field
//   - No legacy field names
//   - RunSummary is the JSON root object
func TestGetRunStatusHandlerCanonicalContract(t *testing.T) {
	runID := types.NewRunID()
	now := time.Now()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID.String(),
			RepoUrl:   "https://github.com/user/repo.git",
			Status:    store.RunStatusRunning,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := getRunStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Canonical contract: HTTP 200.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Parse response as raw JSON to verify no legacy fields are present.
	var rawResp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&rawResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify canonical fields are present.
	if _, ok := rawResp["run_id"]; !ok {
		t.Error("expected 'run_id' field in response (canonical)")
	}
	if _, ok := rawResp["state"]; !ok {
		t.Error("expected 'state' field in response (canonical)")
	}
	if _, ok := rawResp["stages"]; !ok {
		t.Error("expected 'stages' field in response (canonical)")
	}
	if _, ok := rawResp["repository"]; !ok {
		t.Error("expected 'repository' field in response (canonical)")
	}

	// Verify legacy/envelope fields are NOT present.
	if _, ok := rawResp["ticket"]; ok {
		t.Error("unexpected 'ticket' wrapper field in response (legacy)")
	}
	if _, ok := rawResp["ticket_id"]; ok {
		t.Error("unexpected 'ticket_id' field in response (legacy)")
	}
	if _, ok := rawResp["status"]; ok {
		t.Error("unexpected 'status' field in response (legacy - use 'state')")
	}
}

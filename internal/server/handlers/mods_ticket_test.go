package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

func newTestEventsService() *events.Service {
	svc, _ := events.New(events.Options{
		BufferSize:  10,
		HistorySize: 100,
	})
	return svc
}

func TestCreateSingleRepoRunHandler_SingleRepo(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := &mockStore{
		createRunResult: store.Run{
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := createSingleRepoRunHandler(st, nil)

	reqBody := map[string]any{
		"repo_url":   "https://github.com/user/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec": map[string]any{
			"steps": []any{
				map[string]any{"image": "img1:latest"},
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		RunID  string `json:"run_id"`
		ModID  string `json:"mod_id"`
		SpecID string `json:"spec_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.RunID == "" {
		t.Fatalf("expected run_id to be set")
	}
	if resp.ModID == "" {
		t.Fatalf("expected mod_id to be set")
	}
	if resp.SpecID == "" {
		t.Fatalf("expected spec_id to be set")
	}

	if !st.createSpecCalled || !st.createModCalled || !st.createModRepoCalled || !st.createRunCalled || !st.createRunRepoCalled {
		t.Fatalf("expected spec/mod/repo/run creation calls to be made")
	}
	if st.createJobCallCount != 3 {
		t.Fatalf("expected 3 jobs (pre-gate, mod-0, post-gate), got %d", st.createJobCallCount)
	}
	if len(st.createJobParams) != 3 {
		t.Fatalf("expected 3 CreateJob param sets, got %d", len(st.createJobParams))
	}
	if st.createJobParams[0].Status != store.JobStatusQueued {
		t.Fatalf("expected first job to be Queued, got %s", st.createJobParams[0].Status)
	}
	if st.createJobParams[1].Status != store.JobStatusCreated || st.createJobParams[2].Status != store.JobStatusCreated {
		t.Fatalf("expected non-first jobs to be Created, got %s/%s", st.createJobParams[1].Status, st.createJobParams[2].Status)
	}
}

// These tests verify v1 job creation behavior:
// - Jobs are created directly for (runs.id, run_repos.repo_id)
// - jobs.repo_id and jobs.repo_base_ref are persisted correctly
// - First job is Queued, remaining jobs are Created per repo attempt

func TestCreateJobsFromSpec_SingleMod(t *testing.T) {
	t.Parallel()

	runID := domaintypes.RunID("run_test_12345678901234567")
	repoID := domaintypes.ModRepoID("repo_abc")
	const (
		repoBaseRef = "main"
		attempt     = int32(1)
	)

	st := &mockStore{}

	spec := []byte(`{"steps":[{"image":"mod1:v1"}]}`)

	err := createJobsFromSpec(context.Background(), st, runID, repoID, repoBaseRef, attempt, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	// Verify 3 jobs were created: pre-gate, mod-0, post-gate.
	if st.createJobCallCount != 3 {
		t.Fatalf("expected 3 jobs, got %d", st.createJobCallCount)
	}

	// Verify job ordering and status (first job is Queued, rest are Created).
	expectedJobs := []struct {
		name    string
		modType string
		status  store.JobStatus
	}{
		{"pre-gate", "pre_gate", store.JobStatusQueued}, // First job is Queued.
		{"mod-0", "mod", store.JobStatusCreated},        // Remaining jobs are Created.
		{"post-gate", "post_gate", store.JobStatusCreated},
	}

	for i, exp := range expectedJobs {
		got := st.createJobParams[i]
		if got.Name != exp.name {
			t.Errorf("job %d: expected name %q, got %q", i, exp.name, got.Name)
		}
		if got.JobType != exp.modType {
			t.Errorf("job %d: expected mod_type %q, got %q", i, exp.modType, got.JobType)
		}
		if got.Status != exp.status {
			t.Errorf("job %d: expected status %s, got %s", i, exp.status, got.Status)
		}

		// Verify repo_id and repo_base_ref are persisted correctly.
		if got.RepoID != repoID {
			t.Errorf("job %d: expected repo_id %q, got %q", i, repoID, got.RepoID)
		}
		if got.RepoBaseRef != repoBaseRef {
			t.Errorf("job %d: expected repo_base_ref %q, got %q", i, repoBaseRef, got.RepoBaseRef)
		}
		if got.Attempt != attempt {
			t.Errorf("job %d: expected attempt %d, got %d", i, attempt, got.Attempt)
		}
		if got.RunID != runID {
			t.Errorf("job %d: expected run_id %q, got %q", i, runID, got.RunID)
		}
	}
}

func TestCreateJobsFromSpec_MultiStep(t *testing.T) {
	t.Parallel()

	runID := domaintypes.RunID("run_multistep_0123456789")
	repoID := domaintypes.ModRepoID("repo_multi")
	const (
		repoBaseRef = "develop"
		attempt     = int32(2)
	)

	st := &mockStore{}

	// Multi-step spec with 3 steps.
	spec := []byte(`{
		"steps": [
			{"image": "mod1:v1"},
			{"image": "mod2:v2"},
			{"image": "mod3:v3"}
		]
	}`)

	err := createJobsFromSpec(context.Background(), st, runID, repoID, repoBaseRef, attempt, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	// Verify 5 jobs were created: pre-gate, mod-0, mod-1, mod-2, post-gate.
	if st.createJobCallCount != 5 {
		t.Fatalf("expected 5 jobs (pre-gate + 3 mods + post-gate), got %d", st.createJobCallCount)
	}

	// Verify job ordering and status (first job is Queued, rest are Created).
	expectedJobs := []struct {
		name     string
		modType  string
		status   store.JobStatus
		modImage string
	}{
		{"pre-gate", "pre_gate", store.JobStatusQueued, ""}, // First job is Queued.
		{"mod-0", "mod", store.JobStatusCreated, "mod1:v1"}, // Remaining jobs are Created.
		{"mod-1", "mod", store.JobStatusCreated, "mod2:v2"},
		{"mod-2", "mod", store.JobStatusCreated, "mod3:v3"},
		{"post-gate", "post_gate", store.JobStatusCreated, ""},
	}

	for i, exp := range expectedJobs {
		got := st.createJobParams[i]
		if got.Name != exp.name {
			t.Errorf("job %d: expected name %q, got %q", i, exp.name, got.Name)
		}
		if got.JobType != exp.modType {
			t.Errorf("job %d: expected mod_type %q, got %q", i, exp.modType, got.JobType)
		}
		if got.Status != exp.status {
			t.Errorf("job %d: expected status %s, got %s", i, exp.status, got.Status)
		}
		if got.JobImage != exp.modImage {
			t.Errorf("job %d: expected mod_image %q, got %q", i, exp.modImage, got.JobImage)
		}

		// Verify repo_id and repo_base_ref are persisted correctly.
		if got.RepoID != repoID {
			t.Errorf("job %d: expected repo_id %q, got %q", i, repoID, got.RepoID)
		}
		if got.RepoBaseRef != repoBaseRef {
			t.Errorf("job %d: expected repo_base_ref %q, got %q", i, repoBaseRef, got.RepoBaseRef)
		}
		if got.Attempt != attempt {
			t.Errorf("job %d: expected attempt %d, got %d", i, attempt, got.Attempt)
		}
		if got.RunID != runID {
			t.Errorf("job %d: expected run_id %q, got %q", i, runID, got.RunID)
		}
	}
}

func TestJobQueueingRules_FirstJobQueued(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		spec         []byte
		expectedJobs int
	}{
		{
			name:         "single_mod",
			spec:         []byte(`{"steps":[{"image":"a"}]}`),
			expectedJobs: 3,
		},
		{
			name:         "two_mods",
			spec:         []byte(`{"steps":[{"image":"a"},{"image":"b"}]}`),
			expectedJobs: 4,
		},
		{
			name:         "five_mods",
			spec:         []byte(`{"steps":[{"image":"a"},{"image":"b"},{"image":"c"},{"image":"d"},{"image":"e"}]}`),
			expectedJobs: 7,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			st := &mockStore{}

			err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_123"), domaintypes.ModRepoID("repo_456"), "main", 1, tc.spec)
			if err != nil {
				t.Fatalf("createJobsFromSpec failed: %v", err)
			}

			if st.createJobCallCount != tc.expectedJobs {
				t.Fatalf("expected %d jobs, got %d", tc.expectedJobs, st.createJobCallCount)
			}

			// Count jobs with Queued status — exactly one should be Queued.
			queuedCount := 0
			for _, p := range st.createJobParams {
				if p.Status == store.JobStatusQueued {
					queuedCount++
				}
			}

			if queuedCount != 1 {
				t.Errorf("expected exactly 1 Queued job (first job), got %d", queuedCount)
			}

			// Verify the first job is the queue head.
			if st.createJobParams[0].Status != store.JobStatusQueued {
				t.Errorf("expected first job to be Queued, got %s", st.createJobParams[0].Status)
			}

			// Verify all remaining jobs are Created.
			for i := 1; i < len(st.createJobParams); i++ {
				if st.createJobParams[i].Status != store.JobStatusCreated {
					t.Errorf("job %d: expected status Created, got %s", i, st.createJobParams[i].Status)
				}
			}
		})
	}
}

func TestCreateJobsDirectlyForRunRepoID(t *testing.T) {
	t.Parallel()

	runID := domaintypes.RunID("run_v1_direct_addressing_12")
	repoID := domaintypes.ModRepoID("repo_direct_addr")
	const (
		repoBaseRef = "feature/test"
		attempt     = int32(3)
	)

	st := &mockStore{}
	spec := []byte(`{"steps":[{"image":"a"}]}`)

	err := createJobsFromSpec(context.Background(), st, runID, repoID, repoBaseRef, attempt, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	// Verify all jobs are created with the same (run_id, repo_id, attempt) tuple.
	for i, p := range st.createJobParams {
		if p.RunID != runID {
			t.Errorf("job %d: expected run_id %q (direct addressing), got %q", i, runID, p.RunID)
		}
		if p.RepoID != repoID {
			t.Errorf("job %d: expected repo_id %q (no child runs), got %q", i, repoID, p.RepoID)
		}
		if p.Attempt != attempt {
			t.Errorf("job %d: expected attempt %d, got %d", i, attempt, p.Attempt)
		}
		if p.RepoBaseRef != repoBaseRef {
			t.Errorf("job %d: expected repo_base_ref %q, got %q", i, repoBaseRef, p.RepoBaseRef)
		}
	}
}

func TestCreateJobsFromSpec_NextIDChainOrdering(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	spec := []byte(`{"steps":[{"image":"a"},{"image":"b"}]}`)

	err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_123"), domaintypes.ModRepoID("repo_456"), "main", 1, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	// Verify chain ordering: pre-gate -> mod-0 -> mod-1 -> post-gate.
	for i := 0; i < len(st.createJobParams)-1; i++ {
		next := st.createJobParams[i].NextID
		if next == nil {
			t.Fatalf("job %d (%s): expected next_id to be set", i, st.createJobParams[i].Name)
		}
		if *next != st.createJobParams[i+1].ID {
			t.Fatalf("job %d (%s): expected next_id=%s, got %s", i, st.createJobParams[i].Name, st.createJobParams[i+1].ID, *next)
		}
	}
	if st.createJobParams[len(st.createJobParams)-1].NextID != nil {
		t.Fatalf("expected tail job next_id=nil, got %s", *st.createJobParams[len(st.createJobParams)-1].NextID)
	}
}

func TestCreateSingleRepoRunHandler_MissingFields(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := createSingleRepoRunHandler(st, nil)

	cases := []struct {
		name string
		body map[string]any
		err  string
	}{
		{"empty repo_url", map[string]any{"repo_url": "", "base_ref": "main", "target_ref": "feature", "spec": map[string]any{}}, "empty"},
		{"no repo_url", map[string]any{"base_ref": "main", "target_ref": "feature", "spec": map[string]any{}}, "empty"},
		{"empty base_ref", map[string]any{"repo_url": "https://github.com/user/repo.git", "base_ref": "", "target_ref": "feature", "spec": map[string]any{}}, "empty"},
		{"no base_ref", map[string]any{"repo_url": "https://github.com/user/repo.git", "target_ref": "feature", "spec": map[string]any{}}, "empty"},
		{"empty target_ref", map[string]any{"repo_url": "https://github.com/user/repo.git", "base_ref": "main", "target_ref": "", "spec": map[string]any{}}, "empty"},
		{"no target_ref", map[string]any{"repo_url": "https://github.com/user/repo.git", "base_ref": "main", "spec": map[string]any{}}, "empty"},
		{"no spec", map[string]any{"repo_url": "https://github.com/user/repo.git", "base_ref": "main", "target_ref": "feature"}, "spec is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tc.err) {
				t.Fatalf("expected error %q, got: %s", tc.err, rr.Body.String())
			}
		})
	}
}

func TestCreateSingleRepoRunHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := createSingleRepoRunHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", strings.NewReader("{invalid json"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Fatalf("expected 'invalid request' error, got: %s", rr.Body.String())
	}
}

func TestCreateSingleRepoRunHandler_InvalidRepoURL(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := createSingleRepoRunHandler(st, nil)

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
			body := map[string]any{
				"repo_url":   tc.repoURL,
				"base_ref":   "main",
				"target_ref": "feature",
				"spec":       map[string]any{},
			}
			bodyBytes, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(bodyBytes))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tc.errMsg) {
				t.Fatalf("expected error %q, got: %s", tc.errMsg, rr.Body.String())
			}
		})
	}
}

func TestCreateSingleRepoRunHandler_PublishesEvent(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := &mockStore{
		createRunResult: store.Run{
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	eventsService := newTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	reqBody := map[string]any{
		"repo_url":   "https://github.com/user/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec": map[string]any{
			"steps": []any{
				map[string]any{"image": "docker.io/test/mod:latest"},
			},
		},
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
		t.Fatalf("decode response: %v", err)
	}
	runID := resp.RunID

	snapshot := eventsService.Hub().Snapshot(domaintypes.RunID(runID))
	if len(snapshot) == 0 {
		t.Fatalf("expected at least one run event to be published")
	}

	foundRunEvent := false
	for _, evt := range snapshot {
		if evt.Type == domaintypes.SSEEventRun {
			foundRunEvent = true
			if !strings.Contains(string(evt.Data), "\"state\":\"running\"") {
				t.Fatalf("expected run event data to contain state \"running\", got: %s", string(evt.Data))
			}
			break
		}
	}
	if !foundRunEvent {
		t.Fatalf("expected to find a 'run' event in the snapshot")
	}
}

func TestCreateSingleRepoRunHandler_MultiStepCreatesMultipleJobs(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := &mockStore{
		createRunResult: store.Run{
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := createSingleRepoRunHandler(st, nil)

	reqBody := map[string]any{
		"repo_url":   "https://github.com/user/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec": map[string]any{
			"steps": []any{
				map[string]any{"image": "img1:latest"},
				map[string]any{"image": "img2:latest"},
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.createJobCallCount != 4 {
		t.Fatalf("expected 4 jobs (pre-gate, mod-0, mod-1, post-gate), got %d", st.createJobCallCount)
	}
	if st.createJobParams[0].Name != "pre-gate" || st.createJobParams[1].Name != "mod-0" || st.createJobParams[2].Name != "mod-1" || st.createJobParams[3].Name != "post-gate" {
		t.Fatalf("unexpected job ordering: %q, %q, %q, %q", st.createJobParams[0].Name, st.createJobParams[1].Name, st.createJobParams[2].Name, st.createJobParams[3].Name)
	}
	if st.createJobParams[0].Status != store.JobStatusQueued {
		t.Fatalf("expected first job to be Queued, got %s", st.createJobParams[0].Status)
	}
}

func TestGetRunStatusHandler_Success(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	jobID := domaintypes.NewJobID()
	jobIDStr := jobID.String()
	nextJobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		listRunReposByRunResult: []store.RunRepo{
			{RunID: runID, RepoID: "repo_123", RepoBaseRef: "main", RepoTargetRef: "feature"},
		},
		getModRepoResult: store.ModRepo{ID: "repo_123", RepoUrl: "https://github.com/user/repo.git"},
		listJobsByRunResult: []store.Job{
			{ID: jobID, RunID: runID, Status: store.JobStatusQueued, NextID: &nextJobID, Meta: withStepIndexMeta([]byte(`{}`), domaintypes.StepIndex(1000))},
		},
	}

	handler := getRunStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runIDStr+"/status", nil)
	req.SetPathValue("id", runIDStr)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp modsapi.RunSummary
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.RunID.String() != runIDStr {
		t.Fatalf("expected run_id %s, got %s", runIDStr, resp.RunID.String())
	}
	if resp.State != modsapi.RunStateRunning {
		t.Fatalf("expected status running, got %s", resp.State)
	}
	if resp.Repository != "https://github.com/user/repo.git" {
		t.Fatalf("expected repo_url https://github.com/user/repo.git, got %s", resp.Repository)
	}
	if resp.Metadata["repo_base_ref"] != "main" {
		t.Fatalf("expected base_ref main, got %s", resp.Metadata["repo_base_ref"])
	}
	if resp.Metadata["repo_target_ref"] != "feature" {
		t.Fatalf("expected target_ref feature, got %s", resp.Metadata["repo_target_ref"])
	}
	if len(resp.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(resp.Stages))
	}
	if got := resp.Stages[domaintypes.JobID(jobIDStr)].State; got != modsapi.StageStatePending {
		t.Fatalf("expected stage to be pending, got %s", got)
	}
	if got := resp.Stages[domaintypes.JobID(jobIDStr)].NextID; got == nil || *got != nextJobID {
		t.Fatalf("expected stage next_id %s, got %v", nextJobID, got)
	}

	if !st.getRunCalled || !st.listRunReposByRunCalled || !st.getModRepoCalled || !st.listJobsByRunCalled {
		t.Fatalf("expected run status handler to read run+repos+repo_url+jobs")
	}
}

func TestGetRunStatusHandler_NotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()

	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}

	handler := getRunStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/status", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "not found") {
		t.Fatalf("expected 'not found' error, got: %s", rr.Body.String())
	}
}

func TestGetRunStatusHandler_EmptyID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := getRunStatusHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs//status", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "path parameter is required") {
		t.Fatalf("expected required path parameter error, got: %s", rr.Body.String())
	}
}

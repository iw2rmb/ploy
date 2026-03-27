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
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/store"
)

func newTestEventsService() *server.EventsService {
	svc, _ := server.NewEventsService(server.EventsOptions{
		BufferSize:  10,
		HistorySize: 100,
	})
	return svc
}

func createJobsByName(params []store.CreateJobParams) map[string]store.CreateJobParams {
	byName := make(map[string]store.CreateJobParams, len(params))
	for _, p := range params {
		byName[p.Name] = p
	}
	return byName
}

const testRepoSHA0 = "0123456789abcdef0123456789abcdef01234567"

func TestCreateSingleRepoRunHandler_SingleRepo(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := &mockStore{
		createRunResult: store.Run{
			Status:    domaintypes.RunStatusStarted,
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

	assertStatus(t, rr, http.StatusCreated)

	var resp struct {
		RunID  string `json:"run_id"`
		MigID  string `json:"mig_id"`
		SpecID string `json:"spec_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.RunID == "" {
		t.Fatalf("expected run_id to be set")
	}
	if resp.MigID == "" {
		t.Fatalf("expected mig_id to be set")
	}
	if resp.SpecID == "" {
		t.Fatalf("expected spec_id to be set")
	}

	if !st.createSpecCalled || !st.createMigCalled || !st.createMigRepoCalled || !st.createRunCalled || !st.createRunRepoCalled {
		t.Fatalf("expected spec/mig/repo/run creation calls to be made")
	}
	if st.createJobCallCount != 0 {
		t.Fatalf("expected no jobs on submission, got %d", st.createJobCallCount)
	}
}

// These tests verify v1 job creation behavior:
// - Jobs are created directly for (runs.id, run_repos.repo_id)
// - jobs.repo_id and jobs.repo_base_ref are persisted correctly
// - First job is Queued, remaining jobs are Created per repo attempt

func TestCreateJobsFromSpec_SingleMod(t *testing.T) {
	t.Parallel()

	runID := domaintypes.RunID("run_test_12345678901234567")
	repoID := domaintypes.RepoID("repo_abc")
	const (
		repoBaseRef = "main"
		attempt     = int32(1)
	)

	st := &mockStore{}

	spec := []byte(`{"steps":[{"image":"mod1:v1"}]}`)

	err := createJobsFromSpec(context.Background(), st, runID, repoID, repoBaseRef, attempt, testRepoSHA0, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	// Verify 3 jobs were created: pre-gate, mig-0, post-gate.
	if st.createJobCallCount != 3 {
		t.Fatalf("expected 3 jobs, got %d", st.createJobCallCount)
	}

	// Verify job status and shape (pre-gate is Queued, rest are Created).
	expectedJobs := []struct {
		name    string
		jobType domaintypes.JobType
		status  domaintypes.JobStatus
	}{
		{"pre-gate", domaintypes.JobTypePreGate, domaintypes.JobStatusQueued}, // First job is Queued.
		{"mig-0", domaintypes.JobTypeMod, domaintypes.JobStatusCreated},       // Remaining jobs are Created.
		{"post-gate", domaintypes.JobTypePostGate, domaintypes.JobStatusCreated},
	}

	byName := createJobsByName(st.createJobParams)
	for _, exp := range expectedJobs {
		got, ok := byName[exp.name]
		if !ok {
			t.Fatalf("missing job %q", exp.name)
		}
		if got.JobType != exp.jobType {
			t.Errorf("job %q: expected job_type %q, got %q", exp.name, exp.jobType, got.JobType)
		}
		if got.Status != exp.status {
			t.Errorf("job %q: expected status %s, got %s", exp.name, exp.status, got.Status)
		}

		// Verify repo_id and repo_base_ref are persisted correctly.
		if got.RepoID != repoID {
			t.Errorf("job %q: expected repo_id %q, got %q", exp.name, repoID, got.RepoID)
		}
		if got.RepoBaseRef != repoBaseRef {
			t.Errorf("job %q: expected repo_base_ref %q, got %q", exp.name, repoBaseRef, got.RepoBaseRef)
		}
		if got.Attempt != attempt {
			t.Errorf("job %q: expected attempt %d, got %d", exp.name, attempt, got.Attempt)
		}
		if got.RunID != runID {
			t.Errorf("job %q: expected run_id %q, got %q", exp.name, runID, got.RunID)
		}
		if exp.name == "pre-gate" && got.RepoShaIn != testRepoSHA0 {
			t.Errorf("job %q: expected repo_sha_in %q, got %q", exp.name, testRepoSHA0, got.RepoShaIn)
		}
		if exp.name != "pre-gate" && got.RepoShaIn != "" {
			t.Errorf("job %q: expected empty repo_sha_in, got %q", exp.name, got.RepoShaIn)
		}
	}
}

func TestCreateJobsFromSpec_MultiStep(t *testing.T) {
	t.Parallel()

	runID := domaintypes.RunID("run_multistep_0123456789")
	repoID := domaintypes.RepoID("repo_multi")
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

	err := createJobsFromSpec(context.Background(), st, runID, repoID, repoBaseRef, attempt, testRepoSHA0, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	// Verify 5 jobs were created: pre-gate, mig-0, mig-1, mig-2, post-gate.
	if st.createJobCallCount != 5 {
		t.Fatalf("expected 5 jobs (pre-gate + 3 migs + post-gate), got %d", st.createJobCallCount)
	}

	// Verify job status and shape (pre-gate is Queued, rest are Created).
	expectedJobs := []struct {
		name     string
		jobType  domaintypes.JobType
		status   domaintypes.JobStatus
		modImage string
	}{
		{"pre-gate", domaintypes.JobTypePreGate, domaintypes.JobStatusQueued, ""},  // First job is Queued.
		{"mig-0", domaintypes.JobTypeMod, domaintypes.JobStatusCreated, "mod1:v1"}, // Remaining jobs are Created.
		{"mig-1", domaintypes.JobTypeMod, domaintypes.JobStatusCreated, "mod2:v2"},
		{"mig-2", domaintypes.JobTypeMod, domaintypes.JobStatusCreated, "mod3:v3"},
		{"post-gate", domaintypes.JobTypePostGate, domaintypes.JobStatusCreated, ""},
	}

	byName := createJobsByName(st.createJobParams)
	for _, exp := range expectedJobs {
		got, ok := byName[exp.name]
		if !ok {
			t.Fatalf("missing job %q", exp.name)
		}
		if got.JobType != exp.jobType {
			t.Errorf("job %q: expected job_type %q, got %q", exp.name, exp.jobType, got.JobType)
		}
		if got.Status != exp.status {
			t.Errorf("job %q: expected status %s, got %s", exp.name, exp.status, got.Status)
		}
		if got.JobImage != exp.modImage {
			t.Errorf("job %q: expected job_image %q, got %q", exp.name, exp.modImage, got.JobImage)
		}

		// Verify repo_id and repo_base_ref are persisted correctly.
		if got.RepoID != repoID {
			t.Errorf("job %q: expected repo_id %q, got %q", exp.name, repoID, got.RepoID)
		}
		if got.RepoBaseRef != repoBaseRef {
			t.Errorf("job %q: expected repo_base_ref %q, got %q", exp.name, repoBaseRef, got.RepoBaseRef)
		}
		if got.Attempt != attempt {
			t.Errorf("job %q: expected attempt %d, got %d", exp.name, attempt, got.Attempt)
		}
		if got.RunID != runID {
			t.Errorf("job %q: expected run_id %q, got %q", exp.name, runID, got.RunID)
		}
		if exp.name == "pre-gate" && got.RepoShaIn != testRepoSHA0 {
			t.Errorf("job %q: expected repo_sha_in %q, got %q", exp.name, testRepoSHA0, got.RepoShaIn)
		}
		if exp.name != "pre-gate" && got.RepoShaIn != "" {
			t.Errorf("job %q: expected empty repo_sha_in, got %q", exp.name, got.RepoShaIn)
		}
	}
}

func TestCreateJobsFromSpec_InvalidRepoSHA0(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	spec := []byte(`{"steps":[{"image":"a"}]}`)
	err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_123"), domaintypes.RepoID("repo_456"), "main", 1, "not-a-sha", spec)
	if err == nil || !strings.Contains(err.Error(), "repo_sha0 must match") {
		t.Fatalf("expected repo_sha0 validation error, got %v", err)
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

			err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_123"), domaintypes.RepoID("repo_456"), "main", 1, testRepoSHA0, tc.spec)
			if err != nil {
				t.Fatalf("createJobsFromSpec failed: %v", err)
			}

			if st.createJobCallCount != tc.expectedJobs {
				t.Fatalf("expected %d jobs, got %d", tc.expectedJobs, st.createJobCallCount)
			}

			// Count jobs with Queued status — exactly one should be Queued.
			queuedCount := 0
			for _, p := range st.createJobParams {
				if p.Status == domaintypes.JobStatusQueued {
					queuedCount++
				}
			}

			if queuedCount != 1 {
				t.Errorf("expected exactly 1 Queued job (first job), got %d", queuedCount)
			}

			byName := createJobsByName(st.createJobParams)
			if byName["pre-gate"].Status != domaintypes.JobStatusQueued {
				t.Errorf("expected pre-gate to be Queued, got %s", byName["pre-gate"].Status)
			}

			// Verify all non-head jobs are Created.
			for _, p := range st.createJobParams {
				if p.Name == "pre-gate" {
					continue
				}
				if p.Status != domaintypes.JobStatusCreated {
					t.Errorf("job %q: expected status Created, got %s", p.Name, p.Status)
				}
			}
		})
	}
}

func TestCreateJobsDirectlyForRunRepoID(t *testing.T) {
	t.Parallel()

	runID := domaintypes.RunID("run_v1_direct_addressing_12")
	repoID := domaintypes.RepoID("repo_direct_addr")
	const (
		repoBaseRef = "feature/test"
		attempt     = int32(3)
	)

	st := &mockStore{}
	spec := []byte(`{"steps":[{"image":"a"}]}`)

	err := createJobsFromSpec(context.Background(), st, runID, repoID, repoBaseRef, attempt, testRepoSHA0, spec)
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

	err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_123"), domaintypes.RepoID("repo_456"), "main", 1, testRepoSHA0, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	// Verify chain ordering by name: pre-gate -> mig-0 -> mig-1 -> post-gate.
	byName := createJobsByName(st.createJobParams)
	preGate := byName["pre-gate"]
	mig0 := byName["mig-0"]
	mig1 := byName["mig-1"]
	postGate := byName["post-gate"]

	if preGate.NextID == nil || *preGate.NextID != mig0.ID {
		t.Fatalf("pre-gate next_id = %v, want %s", preGate.NextID, mig0.ID)
	}
	if mig0.NextID == nil || *mig0.NextID != mig1.ID {
		t.Fatalf("mig-0 next_id = %v, want %s", mig0.NextID, mig1.ID)
	}
	if mig1.NextID == nil || *mig1.NextID != postGate.ID {
		t.Fatalf("mig-1 next_id = %v, want %s", mig1.NextID, postGate.ID)
	}
	if postGate.NextID != nil {
		t.Fatalf("post-gate next_id = %s, want nil", *postGate.NextID)
	}
}

func TestCreateJobsFromSpec_InsertOrderSatisfiesImmediateNextIDFK(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	spec := []byte(`{"steps":[{"image":"a"},{"image":"b"}]}`)

	err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_123"), domaintypes.RepoID("repo_456"), "main", 1, testRepoSHA0, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	inserted := make(map[domaintypes.JobID]struct{}, len(st.createJobParams))
	for i, p := range st.createJobParams {
		if p.NextID != nil {
			if _, ok := inserted[*p.NextID]; !ok {
				t.Fatalf("insert %d (%s) references next_id %s before it was inserted", i, p.Name, *p.NextID)
			}
		}
		inserted[p.ID] = struct{}{}
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

			assertStatus(t, rr, http.StatusBadRequest)
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

	assertStatus(t, rr, http.StatusBadRequest)
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

			assertStatus(t, rr, http.StatusBadRequest)
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
			Status:    domaintypes.RunStatusStarted,
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
				map[string]any{"image": "docker.io/test/mig:latest"},
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusCreated)

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

func TestCreateSingleRepoRunHandler_MultiStepDefersJobCreation(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := &mockStore{
		createRunResult: store.Run{
			Status:    domaintypes.RunStatusStarted,
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

	assertStatus(t, rr, http.StatusCreated)
	if st.createJobCallCount != 0 {
		t.Fatalf("expected no jobs on submission, got %d", st.createJobCallCount)
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
			Status:    domaintypes.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         runID,
				RepoID:        "repo_123",
				RepoBaseRef:   "main",
				RepoTargetRef: "feature",
				RepoUrl:       "https://github.com/user/repo.git",
			},
		},
		listJobsByRunResult: []store.Job{
			{ID: jobID, RunID: runID, Status: domaintypes.JobStatusQueued, NextID: &nextJobID, Meta: withNextIDMeta([]byte(`{}`), float64(1000))},
		},
	}

	handler := getRunStatusHandler(st)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runIDStr+"/status", nil, "id", runIDStr)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[modsapi.RunSummary](t, rr)

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

	if !st.getRunCalled || !st.listRunReposWithURLByRunCalled || !st.listJobsByRunCalled {
		t.Fatalf("expected run status handler to read run+repos_with_url+jobs")
	}
}

func TestGetRunStatusHandler_NotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()

	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}

	handler := getRunStatusHandler(st)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID+"/status", nil, "id", runID)

	assertStatus(t, rr, http.StatusNotFound)
	if !strings.Contains(rr.Body.String(), "not found") {
		t.Fatalf("expected 'not found' error, got: %s", rr.Body.String())
	}
}

func TestGetRunStatusHandler_EmptyID(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	handler := getRunStatusHandler(st)

	rr := doRequest(t, handler, http.MethodGet, "/v1/runs//status", nil, "id", "")

	assertStatus(t, rr, http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "path parameter is required") {
		t.Fatalf("expected required path parameter error, got: %s", rr.Body.String())
	}
}

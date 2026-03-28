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
	"github.com/iw2rmb/ploy/internal/store"
)


const testRepoSHA0 = "0123456789abcdef0123456789abcdef01234567"

func TestCreateSingleRepoRunHandler_SingleRepo(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := &jobStore{
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

// TestCreateJobsFromSpec verifies v1 job creation behavior:
// - Jobs are created directly for (runs.id, run_repos.repo_id)
// - jobs.repo_id and jobs.repo_base_ref are persisted correctly
// - First job is Queued, remaining jobs are Created per repo attempt
func TestCreateJobsFromSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		runID       domaintypes.RunID
		repoID      domaintypes.RepoID
		repoBaseRef string
		attempt     int32
		spec        []byte
		expected    []expectedJob
	}{
		{
			name:        "SingleMod",
			runID:       domaintypes.RunID("run_test_12345678901234567"),
			repoID:      domaintypes.RepoID("repo_abc"),
			repoBaseRef: "main",
			attempt:     1,
			spec:        []byte(`{"steps":[{"image":"mod1:v1"}]}`),
			expected: []expectedJob{
				{"pre-gate", domaintypes.JobTypePreGate, domaintypes.JobStatusQueued, "", testRepoSHA0},
				{"mig-0", domaintypes.JobTypeMod, domaintypes.JobStatusCreated, "", ""},
				{"post-gate", domaintypes.JobTypePostGate, domaintypes.JobStatusCreated, "", ""},
			},
		},
		{
			name:        "MultiStep",
			runID:       domaintypes.RunID("run_multistep_0123456789"),
			repoID:      domaintypes.RepoID("repo_multi"),
			repoBaseRef: "develop",
			attempt:     2,
			spec:        []byte(`{"steps":[{"image":"mod1:v1"},{"image":"mod2:v2"},{"image":"mod3:v3"}]}`),
			expected: []expectedJob{
				{"pre-gate", domaintypes.JobTypePreGate, domaintypes.JobStatusQueued, "", testRepoSHA0},
				{"mig-0", domaintypes.JobTypeMod, domaintypes.JobStatusCreated, "mod1:v1", ""},
				{"mig-1", domaintypes.JobTypeMod, domaintypes.JobStatusCreated, "mod2:v2", ""},
				{"mig-2", domaintypes.JobTypeMod, domaintypes.JobStatusCreated, "mod3:v3", ""},
				{"post-gate", domaintypes.JobTypePostGate, domaintypes.JobStatusCreated, "", ""},
			},
		},
		{
			name:        "CustomAttemptAndRef",
			runID:       domaintypes.RunID("run_v1_direct_addressing_12"),
			repoID:      domaintypes.RepoID("repo_direct_addr"),
			repoBaseRef: "feature/test",
			attempt:     3,
			spec:        []byte(`{"steps":[{"image":"a"}]}`),
			expected: []expectedJob{
				{"pre-gate", domaintypes.JobTypePreGate, domaintypes.JobStatusQueued, "", testRepoSHA0},
				{"mig-0", domaintypes.JobTypeMod, domaintypes.JobStatusCreated, "", ""},
				{"post-gate", domaintypes.JobTypePostGate, domaintypes.JobStatusCreated, "", ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &jobStore{}

			err := createJobsFromSpec(context.Background(), st, tt.runID, tt.repoID, tt.repoBaseRef, tt.attempt, testRepoSHA0, tt.spec)
			if err != nil {
				t.Fatalf("createJobsFromSpec failed: %v", err)
			}

			assertJobChain(t, st.createJobParams, tt.runID, tt.repoID, tt.repoBaseRef, tt.attempt, tt.expected)
		})
	}
}

func TestCreateJobsFromSpec_InvalidRepoSHA0(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
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
			st := &jobStore{}

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

func TestCreateJobsFromSpec_NextIDChainOrdering(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
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

	st := &jobStore{}
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

	st := &jobStore{}
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

	st := &jobStore{}
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

	st := &jobStore{}
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
	st := &jobStore{
		createRunResult: store.Run{
			Status:    domaintypes.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	eventsService, _ := createTestEventsService()
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

func TestGetRunStatusHandler_Success(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	jobID := domaintypes.NewJobID()
	jobIDStr := jobID.String()
	nextJobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &jobStore{
		listJobsByRunResult: []store.Job{
			{ID: jobID, RunID: runID, Status: domaintypes.JobStatusQueued, NextID: &nextJobID, Meta: withNextIDMeta([]byte(`{}`), float64(1000))},
		},
	}
	st.getRun.val = store.Run{
		ID:        runID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		}
	st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{
		{
			RunID:         runID,
			RepoID:        "repo_123",
			RepoBaseRef:   "main",
			RepoTargetRef: "feature",
			RepoUrl:       "https://github.com/user/repo.git",
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

	if !st.getRun.called || !st.listRunReposWithURLByRun.called || !st.listJobsByRunCalled {
		t.Fatalf("expected run status handler to read run+repos_with_url+jobs")
	}
}

func TestGetRunStatusHandler_NotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID().String()

	st := &jobStore{}
	st.getRun.err = pgx.ErrNoRows

	handler := getRunStatusHandler(st)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID+"/status", nil, "id", runID)

	assertStatus(t, rr, http.StatusNotFound)
	if !strings.Contains(rr.Body.String(), "not found") {
		t.Fatalf("expected 'not found' error, got: %s", rr.Body.String())
	}
}

func TestGetRunStatusHandler_EmptyID(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
	handler := getRunStatusHandler(st)

	rr := doRequest(t, handler, http.MethodGet, "/v1/runs//status", nil, "id", "")

	assertStatus(t, rr, http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "path parameter is required") {
		t.Fatalf("expected required path parameter error, got: %s", rr.Body.String())
	}
}

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListReposHandler_Success_Empty(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		listDistinctReposResult: []store.ListDistinctReposRow{},
	}
	handler := listReposHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Repos []RepoSummary `json:"repos"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Repos) != 0 {
		t.Fatalf("expected 0 repos, got %d", len(resp.Repos))
	}

	if !st.listDistinctReposCalled {
		t.Fatalf("expected ListDistinctRepos to be called")
	}
	if st.listDistinctReposParam != "" {
		t.Fatalf("expected empty filter, got %q", st.listDistinctReposParam)
	}
}

func TestListReposHandler_Success_WithData(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	failureCode := "BUILD_FAILED"
	st := &mockStore{
		listDistinctReposResult: []store.ListDistinctReposRow{
			{
				RepoID:        "repo0001",
				RepoUrl:       "https://github.com/org/repo1.git",
				LastRunAt:     pgtype.Timestamptz{Time: now, Valid: true},
				LastStatus:    "Success",
				PrepStatus:    store.PrepStatusReady,
				PrepUpdatedAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true},
			},
			{
				RepoID:          "repo0002",
				RepoUrl:         "https://github.com/org/repo2.git",
				LastRunAt:       pgtype.Timestamptz{Valid: false},
				LastStatus:      "",
				PrepStatus:      store.PrepStatusFailed,
				PrepFailureCode: &failureCode,
				PrepUpdatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
			},
		},
	}
	handler := listReposHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Repos []RepoSummary `json:"repos"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(resp.Repos))
	}

	if resp.Repos[0].RepoID.String() != "repo0001" || resp.Repos[0].RepoURL != "https://github.com/org/repo1.git" {
		t.Fatalf("unexpected repo[0]: %+v", resp.Repos[0])
	}
	if resp.Repos[0].LastRunAt == nil {
		t.Fatalf("expected last_run_at to be set for repo[0]")
	}
	if resp.Repos[0].LastStatus == nil || *resp.Repos[0].LastStatus != "Success" {
		t.Fatalf("expected last_status Success, got %v", resp.Repos[0].LastStatus)
	}
	if resp.Repos[0].PrepStatus != string(store.PrepStatusReady) {
		t.Fatalf("expected prep_status %q, got %q", store.PrepStatusReady, resp.Repos[0].PrepStatus)
	}
	if resp.Repos[0].PrepUpdatedAt == nil {
		t.Fatalf("expected prep_updated_at to be set for repo[0]")
	}

	if resp.Repos[1].RepoID.String() != "repo0002" || resp.Repos[1].RepoURL != "https://github.com/org/repo2.git" {
		t.Fatalf("unexpected repo[1]: %+v", resp.Repos[1])
	}
	if resp.Repos[1].LastRunAt != nil {
		t.Fatalf("expected last_run_at to be nil for repo[1]")
	}
	if resp.Repos[1].LastStatus != nil {
		t.Fatalf("expected last_status to be nil for repo[1], got %v", resp.Repos[1].LastStatus)
	}
	if resp.Repos[1].PrepStatus != string(store.PrepStatusFailed) {
		t.Fatalf("expected prep_status %q, got %q", store.PrepStatusFailed, resp.Repos[1].PrepStatus)
	}
	if resp.Repos[1].PrepFailureCode == nil || *resp.Repos[1].PrepFailureCode != "BUILD_FAILED" {
		t.Fatalf("expected prep_failure_code BUILD_FAILED, got %v", resp.Repos[1].PrepFailureCode)
	}
}

func TestListReposHandler_WithFilter(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		listDistinctReposResult: []store.ListDistinctReposRow{},
	}
	handler := listReposHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos?contains=org/project", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if st.listDistinctReposParam != "org/project" {
		t.Fatalf("expected filter 'org/project', got %q", st.listDistinctReposParam)
	}
}

func TestListReposHandler_StoreError(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		listDistinctReposErr: errors.New("database connection failed"),
	}
	handler := listReposHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}

func TestListRunsForRepoHandler_Success(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	runID := domaintypes.NewRunID()
	modID := domaintypes.NewMigID()
	st := &mockStore{
		listRunsForRepoResult: []store.ListRunsForRepoRow{
			{
				RunID:         runID,
				MigID:         modID,
				RunStatus:     store.RunStatusFinished,
				RepoStatus:    store.RunRepoStatusSuccess,
				RepoBaseRef:   "main",
				RepoTargetRef: "feature-branch",
				Attempt:       1,
				StartedAt:     pgtype.Timestamptz{Time: now, Valid: true},
				FinishedAt:    pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
			},
		},
	}
	handler := listRunsForRepoHandler(st)

	repoID := "repo_123"
	req := httptest.NewRequest(http.MethodGet, "/v1/repos/"+repoID+"/runs", nil)
	req.SetPathValue("repo_id", repoID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Runs []RepoRunSummary `json:"runs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(resp.Runs))
	}

	run := resp.Runs[0]
	if run.RunID.String() != runID.String() {
		t.Fatalf("unexpected run_id: %s", run.RunID.String())
	}
	if run.MigID != modID {
		t.Fatalf("unexpected mig_id: %s", run.MigID.String())
	}
	if run.RunStatus != "Finished" {
		t.Fatalf("unexpected run_status: %s", run.RunStatus)
	}
	if run.RepoStatus != "Success" {
		t.Fatalf("unexpected repo_status: %s", run.RepoStatus)
	}
	if run.BaseRef != "main" {
		t.Fatalf("unexpected base_ref: %s", run.BaseRef)
	}
	if run.TargetRef != "feature-branch" {
		t.Fatalf("unexpected target_ref: %s", run.TargetRef)
	}
	if run.Attempt != 1 {
		t.Fatalf("unexpected attempt: %d", run.Attempt)
	}

	if !st.listRunsForRepoCalled {
		t.Fatalf("expected ListRunsForRepo to be called")
	}
	if st.listRunsForRepoParams.RepoID.String() != repoID {
		t.Fatalf("expected repo_id %q, got %q", repoID, st.listRunsForRepoParams.RepoID.String())
	}
	if st.listRunsForRepoParams.Limit != 50 {
		t.Fatalf("expected default limit 50, got %d", st.listRunsForRepoParams.Limit)
	}
	if st.listRunsForRepoParams.Offset != 0 {
		t.Fatalf("expected default offset 0, got %d", st.listRunsForRepoParams.Offset)
	}
}

func TestListRunsForRepoHandler_WithPagination(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		listRunsForRepoResult: []store.ListRunsForRepoRow{},
	}
	handler := listRunsForRepoHandler(st)

	repoID := "repo_123"
	req := httptest.NewRequest(http.MethodGet, "/v1/repos/"+repoID+"/runs?limit=25&offset=10", nil)
	req.SetPathValue("repo_id", repoID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	if st.listRunsForRepoParams.Limit != 25 {
		t.Fatalf("expected limit 25, got %d", st.listRunsForRepoParams.Limit)
	}
	if st.listRunsForRepoParams.Offset != 10 {
		t.Fatalf("expected offset 10, got %d", st.listRunsForRepoParams.Offset)
	}
}

func TestListRunsForRepoHandler_InvalidPagination(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		url  string
	}{
		{"invalid limit", "/v1/repos/repo_123/runs?limit=0"},
		{"invalid offset", "/v1/repos/repo_123/runs?offset=-1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &mockStore{listRunsForRepoErr: errors.New("should not be called")}
			handler := listRunsForRepoHandler(st)

			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			req.SetPathValue("repo_id", "repo_123")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", rr.Code)
			}
		})
	}
}

func TestListRunsForRepoHandler_MissingRepoID(t *testing.T) {
	t.Parallel()

	st := &mockStore{listRunsForRepoErr: errors.New("should not be called")}
	handler := listRunsForRepoHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos//runs", nil)
	req.SetPathValue("repo_id", "")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestGetRepoPrepHandler_Success(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	repoID := domaintypes.MigRepoID("repo_123")
	lastErr := "failed to compile"
	failureCode := "BUILD_FAILED"
	logRef := "blob://prep-run/2"

	st := &mockStore{
		getModRepoResult: store.MigRepo{
			ID:              repoID,
			PrepStatus:      store.PrepStatusFailed,
			PrepAttempts:    2,
			PrepLastError:   &lastErr,
			PrepFailureCode: &failureCode,
			PrepUpdatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
			PrepProfile:     []byte(`{"schema_version":1}`),
			PrepArtifacts:   []byte(`{"logs":["blob://prep-run/2"]}`),
		},
		listPrepRunsByRepoResult: []store.PrepRun{
			{
				RepoID:     repoID,
				Attempt:    2,
				Status:     store.PrepStatusFailed,
				StartedAt:  pgtype.Timestamptz{Time: now.Add(-2 * time.Minute), Valid: true},
				FinishedAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true},
				ResultJson: []byte(`{"error":"build failed"}`),
				LogsRef:    &logRef,
			},
			{
				RepoID:     repoID,
				Attempt:    1,
				Status:     store.PrepStatusReady,
				StartedAt:  pgtype.Timestamptz{Time: now.Add(-4 * time.Minute), Valid: true},
				FinishedAt: pgtype.Timestamptz{Time: now.Add(-3 * time.Minute), Valid: true},
				ResultJson: []byte(`{"ok":true}`),
			},
		},
	}
	handler := getRepoPrepHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos/"+repoID.String()+"/prep", nil)
	req.SetPathValue("repo_id", repoID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp RepoPrepSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.RepoID != repoID {
		t.Fatalf("unexpected repo_id: got=%q want=%q", resp.RepoID, repoID)
	}
	if resp.PrepStatus != string(store.PrepStatusFailed) {
		t.Fatalf("unexpected prep_status: got=%q want=%q", resp.PrepStatus, store.PrepStatusFailed)
	}
	if resp.PrepAttempts != 2 {
		t.Fatalf("unexpected prep_attempts: got=%d want=2", resp.PrepAttempts)
	}
	if resp.PrepFailureCode == nil || *resp.PrepFailureCode != failureCode {
		t.Fatalf("unexpected prep_failure_code: got=%v want=%q", resp.PrepFailureCode, failureCode)
	}
	if string(resp.PrepProfile) != `{"schema_version":1}` {
		t.Fatalf("unexpected prep_profile: %s", string(resp.PrepProfile))
	}
	if string(resp.PrepArtifacts) != `{"logs":["blob://prep-run/2"]}` {
		t.Fatalf("unexpected prep_artifacts: %s", string(resp.PrepArtifacts))
	}
	if len(resp.Runs) != 2 {
		t.Fatalf("unexpected run count: got=%d want=2", len(resp.Runs))
	}
	if resp.Runs[0].Attempt != 2 {
		t.Fatalf("unexpected first run attempt: got=%d want=2", resp.Runs[0].Attempt)
	}
	if string(resp.Runs[0].ResultJSON) != `{"error":"build failed"}` {
		t.Fatalf("unexpected first run result_json: %s", string(resp.Runs[0].ResultJSON))
	}

	if !st.getModRepoCalled {
		t.Fatalf("expected GetMigRepo to be called")
	}
	if !st.listPrepRunsByRepoCalled {
		t.Fatalf("expected ListPrepRunsByRepo to be called")
	}
	if st.listPrepRunsByRepoParam != repoID {
		t.Fatalf("unexpected ListPrepRunsByRepo repo_id: got=%q want=%q", st.listPrepRunsByRepoParam, repoID)
	}
}

func TestGetRepoPrepHandler_RepoNotFound(t *testing.T) {
	t.Parallel()

	st := &mockStore{getModRepoErr: pgx.ErrNoRows}
	handler := getRepoPrepHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos/repo_123/prep", nil)
	req.SetPathValue("repo_id", "repo_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if st.listPrepRunsByRepoCalled {
		t.Fatalf("expected ListPrepRunsByRepo not to be called")
	}
}

func TestGetRepoPrepHandler_ListPrepRunsError(t *testing.T) {
	t.Parallel()

	repoID := domaintypes.MigRepoID("repo_123")
	st := &mockStore{
		getModRepoResult: store.MigRepo{
			ID:         repoID,
			PrepStatus: store.PrepStatusReady,
		},
		listPrepRunsByRepoErr: errors.New("db timeout"),
	}
	handler := getRepoPrepHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos/repo_123/prep", nil)
	req.SetPathValue("repo_id", repoID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}

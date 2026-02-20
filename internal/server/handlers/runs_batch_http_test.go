package handlers

import (
	"bytes"
	"context"
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

type cancelRunStoreMock struct {
	store.Store
	getRunResult store.Run
	getRunErr    error

	cancelRunV1Called bool
	cancelRunV1Param  domaintypes.RunID
	cancelRunV1Err    error

	countRows []store.CountRunReposByStatusRow
	countErr  error
}

func (m *cancelRunStoreMock) GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error) {
	return m.getRunResult, m.getRunErr
}

func (m *cancelRunStoreMock) CancelRunV1(ctx context.Context, runID domaintypes.RunID) error {
	m.cancelRunV1Called = true
	m.cancelRunV1Param = runID
	return m.cancelRunV1Err
}

func (m *cancelRunStoreMock) CountRunReposByStatus(ctx context.Context, runID domaintypes.RunID) ([]store.CountRunReposByStatusRow, error) {
	return m.countRows, m.countErr
}

type addRunRepoStoreMock struct {
	store.Store
	getRunResult store.Run
	getRunErr    error

	createModRepoCalled bool
	createModRepoParams store.CreateModRepoParams
	createModRepoResult store.ModRepo
	createModRepoErr    error

	createRunRepoCalled bool
	createRunRepoParams store.CreateRunRepoParams
	createRunRepoResult store.RunRepo
	createRunRepoErr    error

	getSpecResult store.Spec
	getSpecErr    error

	createJobCallCount int
	createJobParams    []store.CreateJobParams
	createJobErr       error
}

func (m *addRunRepoStoreMock) GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error) {
	return m.getRunResult, m.getRunErr
}

func (m *addRunRepoStoreMock) CreateModRepo(ctx context.Context, params store.CreateModRepoParams) (store.ModRepo, error) {
	m.createModRepoCalled = true
	m.createModRepoParams = params
	return m.createModRepoResult, m.createModRepoErr
}

func (m *addRunRepoStoreMock) CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error) {
	m.createRunRepoCalled = true
	m.createRunRepoParams = params
	if m.createRunRepoErr != nil {
		return store.RunRepo{}, m.createRunRepoErr
	}
	result := m.createRunRepoResult
	if result.RunID.IsZero() {
		result.RunID = params.RunID
	}
	if result.RepoID.IsZero() {
		result.RepoID = params.RepoID
	}
	if result.ModID.IsZero() {
		result.ModID = params.ModID
	}
	if result.RepoBaseRef == "" {
		result.RepoBaseRef = params.RepoBaseRef
	}
	if result.RepoTargetRef == "" {
		result.RepoTargetRef = params.RepoTargetRef
	}
	if result.Status == "" {
		result.Status = store.RunRepoStatusQueued
	}
	if result.Attempt == 0 {
		result.Attempt = 1
	}
	if !result.CreatedAt.Valid {
		result.CreatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}
	return result, nil
}

func (m *addRunRepoStoreMock) GetSpec(ctx context.Context, id domaintypes.SpecID) (store.Spec, error) {
	return m.getSpecResult, m.getSpecErr
}

func (m *addRunRepoStoreMock) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJobCallCount++
	m.createJobParams = append(m.createJobParams, params)
	if m.createJobErr != nil {
		return store.Job{}, m.createJobErr
	}
	return store.Job{ID: domaintypes.NewJobID()}, nil
}

type listRunReposStoreMock struct {
	store.Store
	listRunReposWithURLByRunCalled bool
	listRunReposWithURLByRunParam  domaintypes.RunID
	listRunReposWithURLByRunResult []store.ListRunReposWithURLByRunRow
	listRunReposWithURLByRunErr    error
}

func (m *listRunReposStoreMock) ListRunReposWithURLByRun(ctx context.Context, runID domaintypes.RunID) ([]store.ListRunReposWithURLByRunRow, error) {
	m.listRunReposWithURLByRunCalled = true
	m.listRunReposWithURLByRunParam = runID
	return m.listRunReposWithURLByRunResult, m.listRunReposWithURLByRunErr
}

type cancelRunRepoStoreMock struct {
	store.Store
	getRunRepoResults []store.RunRepo
	getRunRepoErr     error
	getRunRepoCalls   int

	getModRepoResult store.ModRepo
	getModRepoErr    error

	updateRunRepoStatusCalled bool
	updateRunRepoStatusParams []store.UpdateRunRepoStatusParams

	listJobsResult []store.Job
	listJobsErr    error

	updateJobStatusCalled bool
	updateJobStatusCalls  []store.UpdateJobStatusParams
}

func (m *cancelRunRepoStoreMock) GetRunRepo(ctx context.Context, arg store.GetRunRepoParams) (store.RunRepo, error) {
	if m.getRunRepoErr != nil {
		return store.RunRepo{}, m.getRunRepoErr
	}
	if len(m.getRunRepoResults) == 0 {
		return store.RunRepo{}, nil
	}
	idx := m.getRunRepoCalls
	if idx >= len(m.getRunRepoResults) {
		idx = len(m.getRunRepoResults) - 1
	}
	m.getRunRepoCalls++
	return m.getRunRepoResults[idx], nil
}

func (m *cancelRunRepoStoreMock) GetModRepo(ctx context.Context, id domaintypes.ModRepoID) (store.ModRepo, error) {
	return m.getModRepoResult, m.getModRepoErr
}

func (m *cancelRunRepoStoreMock) UpdateRunRepoStatus(ctx context.Context, params store.UpdateRunRepoStatusParams) error {
	m.updateRunRepoStatusCalled = true
	m.updateRunRepoStatusParams = append(m.updateRunRepoStatusParams, params)
	return nil
}

func (m *cancelRunRepoStoreMock) ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	return m.listJobsResult, m.listJobsErr
}

func (m *cancelRunRepoStoreMock) UpdateJobStatus(ctx context.Context, params store.UpdateJobStatusParams) error {
	m.updateJobStatusCalled = true
	m.updateJobStatusCalls = append(m.updateJobStatusCalls, params)
	return nil
}

type restartRunRepoStoreMock struct {
	store.Store
	getRunResult store.Run
	getRunErr    error

	getRunRepoResults []store.RunRepo
	getRunRepoErr     error
	getRunRepoCalls   int

	updateRunStatusCalled bool
	updateRunStatusParams []store.UpdateRunStatusParams
	updateRunStatusErr    error

	updateRunRepoRefsCalled bool
	updateRunRepoRefsParams []store.UpdateRunRepoRefsParams

	updateModRepoRefsCalled bool
	updateModRepoRefsParams []store.UpdateModRepoRefsParams

	incrementRunRepoAttemptCalled bool
	incrementRunRepoAttemptParam  store.IncrementRunRepoAttemptParams
	incrementRunRepoAttemptErr    error

	getSpecResult store.Spec
	getSpecErr    error

	createJobCallCount int
	createJobParams    []store.CreateJobParams
	createJobErr       error

	getModRepoResult store.ModRepo
	getModRepoErr    error
}

func (m *restartRunRepoStoreMock) GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error) {
	return m.getRunResult, m.getRunErr
}

func (m *restartRunRepoStoreMock) GetRunRepo(ctx context.Context, arg store.GetRunRepoParams) (store.RunRepo, error) {
	if m.getRunRepoErr != nil {
		return store.RunRepo{}, m.getRunRepoErr
	}
	if len(m.getRunRepoResults) == 0 {
		return store.RunRepo{}, nil
	}
	idx := m.getRunRepoCalls
	if idx >= len(m.getRunRepoResults) {
		idx = len(m.getRunRepoResults) - 1
	}
	m.getRunRepoCalls++
	return m.getRunRepoResults[idx], nil
}

func (m *restartRunRepoStoreMock) UpdateRunStatus(ctx context.Context, params store.UpdateRunStatusParams) error {
	m.updateRunStatusCalled = true
	m.updateRunStatusParams = append(m.updateRunStatusParams, params)
	return m.updateRunStatusErr
}

func (m *restartRunRepoStoreMock) UpdateRunRepoRefs(ctx context.Context, params store.UpdateRunRepoRefsParams) error {
	m.updateRunRepoRefsCalled = true
	m.updateRunRepoRefsParams = append(m.updateRunRepoRefsParams, params)
	return nil
}

func (m *restartRunRepoStoreMock) UpdateModRepoRefs(ctx context.Context, params store.UpdateModRepoRefsParams) error {
	m.updateModRepoRefsCalled = true
	m.updateModRepoRefsParams = append(m.updateModRepoRefsParams, params)
	return nil
}

func (m *restartRunRepoStoreMock) IncrementRunRepoAttempt(ctx context.Context, arg store.IncrementRunRepoAttemptParams) error {
	m.incrementRunRepoAttemptCalled = true
	m.incrementRunRepoAttemptParam = arg
	return m.incrementRunRepoAttemptErr
}

func (m *restartRunRepoStoreMock) GetSpec(ctx context.Context, id domaintypes.SpecID) (store.Spec, error) {
	return m.getSpecResult, m.getSpecErr
}

func (m *restartRunRepoStoreMock) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJobCallCount++
	m.createJobParams = append(m.createJobParams, params)
	if m.createJobErr != nil {
		return store.Job{}, m.createJobErr
	}
	return store.Job{ID: domaintypes.NewJobID()}, nil
}

func (m *restartRunRepoStoreMock) GetModRepo(ctx context.Context, id domaintypes.ModRepoID) (store.ModRepo, error) {
	return m.getModRepoResult, m.getModRepoErr
}

type startRunStoreMock struct {
	store.Store
	getRunResult store.Run
	getRunErr    error

	getSpecResult store.Spec
	getSpecErr    error

	listRunReposByRunResult []store.RunRepo
	listRunReposByRunErr    error

	listQueuedRunReposByRunResult []store.RunRepo
	listQueuedRunReposByRunErr    error

	listJobsByRunRepoAttemptResult []store.Job
	listJobsByRunRepoAttemptErr    error

	updateRunRepoErrorCalled bool
	updateRunRepoErrorParams []store.UpdateRunRepoErrorParams

	scheduleNextJobCalled bool
	scheduleNextJobParam  store.ScheduleNextJobParams
	scheduleNextJobResult store.Job
	scheduleNextJobErr    error

	createJobCallCount int
	createJobParams    []store.CreateJobParams
	createJobErr       error
}

func (m *startRunStoreMock) GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error) {
	return m.getRunResult, m.getRunErr
}

func (m *startRunStoreMock) GetSpec(ctx context.Context, id domaintypes.SpecID) (store.Spec, error) {
	return m.getSpecResult, m.getSpecErr
}

func (m *startRunStoreMock) ListRunReposByRun(ctx context.Context, runID domaintypes.RunID) ([]store.RunRepo, error) {
	return m.listRunReposByRunResult, m.listRunReposByRunErr
}

func (m *startRunStoreMock) ListQueuedRunReposByRun(ctx context.Context, runID domaintypes.RunID) ([]store.RunRepo, error) {
	return m.listQueuedRunReposByRunResult, m.listQueuedRunReposByRunErr
}

func (m *startRunStoreMock) ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	return m.listJobsByRunRepoAttemptResult, m.listJobsByRunRepoAttemptErr
}

func (m *startRunStoreMock) UpdateRunRepoError(ctx context.Context, params store.UpdateRunRepoErrorParams) error {
	m.updateRunRepoErrorCalled = true
	m.updateRunRepoErrorParams = append(m.updateRunRepoErrorParams, params)
	return nil
}

func (m *startRunStoreMock) ScheduleNextJob(ctx context.Context, arg store.ScheduleNextJobParams) (store.Job, error) {
	m.scheduleNextJobCalled = true
	m.scheduleNextJobParam = arg
	return m.scheduleNextJobResult, m.scheduleNextJobErr
}

func (m *startRunStoreMock) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJobCallCount++
	m.createJobParams = append(m.createJobParams, params)
	if m.createJobErr != nil {
		return store.Job{}, m.createJobErr
	}
	return store.Job{ID: domaintypes.NewJobID()}, nil
}

func TestCancelRunHandlerV1_CancelsRunAndWork(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &cancelRunStoreMock{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     domaintypes.NewModID(),
			SpecID:    domaintypes.NewSpecID(),
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/cancel", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.cancelRunV1Called {
		t.Fatalf("expected CancelRunV1 to be called")
	}
	if st.cancelRunV1Param != runID {
		t.Fatalf("expected CancelRunV1 run id %q, got %q", runID, st.cancelRunV1Param)
	}
}

func TestCancelRunHandlerV1_CancelRunV1Error(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &cancelRunStoreMock{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     domaintypes.NewModID(),
			SpecID:    domaintypes.NewSpecID(),
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		cancelRunV1Err: errors.New("db exploded"),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/cancel", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.cancelRunV1Called {
		t.Fatalf("expected CancelRunV1 to be called")
	}
}

func TestCancelRunHandlerV1_TerminalRunIsIdempotent(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &cancelRunStoreMock{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     domaintypes.NewModID(),
			SpecID:    domaintypes.NewSpecID(),
			Status:    store.RunStatusCancelled,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/cancel", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.cancelRunV1Called {
		t.Fatalf("did not expect CancelRunV1 to be called for terminal run")
	}
}

func TestAddRunRepoHandler_CreatesRepoAndJobs(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()

	st := &addRunRepoStoreMock{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     domaintypes.NewModID(),
			SpecID:    specID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
		createModRepoResult: store.ModRepo{
			ID:        repoID,
			RepoUrl:   "https://github.com/org/repo.git",
			BaseRef:   "main",
			TargetRef: "feature",
		},
	}

	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/repos", bytes.NewReader(body))
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	addRunRepoHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.createModRepoCalled || !st.createRunRepoCalled {
		t.Fatalf("expected CreateModRepo and CreateRunRepo to be called")
	}
	if st.createJobCallCount != 3 {
		t.Fatalf("expected 3 jobs to be created for new repo, got %d", st.createJobCallCount)
	}
}

func TestListRunReposHandler_Success(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()

	st := &listRunReposStoreMock{
		listRunReposWithURLByRunResult: []store.ListRunReposWithURLByRunRow{
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoBaseRef:   "main",
				RepoTargetRef: "feature",
				Status:        store.RunRepoStatusQueued,
				Attempt:       1,
				CreatedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				RepoUrl:       "https://github.com/org/repo.git",
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	listRunReposHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Repos []RunRepoResponse `json:"repos"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Repos) != 1 || resp.Repos[0].RepoID != repoID || resp.Repos[0].RepoURL != "https://github.com/org/repo.git" {
		t.Fatalf("unexpected repos response: %+v", resp.Repos)
	}
	if !st.listRunReposWithURLByRunCalled {
		t.Fatalf("expected ListRunReposWithURLByRun to be called")
	}
	if st.listRunReposWithURLByRunParam != runID {
		t.Fatalf("expected run id %q, got %q", runID, st.listRunReposWithURLByRunParam)
	}
}

func TestListRunReposHandler_ListError(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &listRunReposStoreMock{
		listRunReposWithURLByRunErr: errors.New("db exploded"),
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	listRunReposHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCancelRunRepoHandlerV1_NotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	st := &cancelRunRepoStoreMock{
		getRunRepoErr: pgx.ErrNoRows,
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/cancel", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoID.String())
	rr := httptest.NewRecorder()

	cancelRunRepoHandlerV1(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

func TestRestartRunRepoHandler_ReopensTerminalRunAndCreatesJobs(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()

	st := &restartRunRepoStoreMock{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     domaintypes.NewModID(),
			SpecID:    specID,
			Status:    store.RunStatusFinished,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		getRunRepoResults: []store.RunRepo{
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoBaseRef:   "main",
				RepoTargetRef: "feature",
				Attempt:       1,
				Status:        store.RunRepoStatusFail,
				CreatedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			},
			{
				RunID:         runID,
				RepoID:        repoID,
				RepoBaseRef:   "develop",
				RepoTargetRef: "feature-2",
				Attempt:       2,
				Status:        store.RunRepoStatusQueued,
				CreatedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			},
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
		getModRepoResult: store.ModRepo{
			ID:      repoID,
			RepoUrl: "https://github.com/org/repo.git",
		},
	}

	reqBody := map[string]any{
		"base_ref":   "develop",
		"target_ref": "feature-2",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/restart", bytes.NewReader(body))
	req.SetPathValue("id", runID.String())
	req.SetPathValue("repo_id", repoID.String())
	rr := httptest.NewRecorder()

	restartRunRepoHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateRunStatusCalled {
		t.Fatalf("expected UpdateRunStatus to be called for terminal run")
	}
	if !st.updateRunRepoRefsCalled || !st.updateModRepoRefsCalled {
		t.Fatalf("expected refs updates to be called")
	}
	if !st.incrementRunRepoAttemptCalled {
		t.Fatalf("expected IncrementRunRepoAttempt to be called")
	}
	if st.createJobCallCount != 3 {
		t.Fatalf("expected 3 jobs for restarted repo, got %d", st.createJobCallCount)
	}
}

func TestStartRunHandler_StartsQueuedRepos(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()

	queuedRepo := store.RunRepo{
		RunID:         runID,
		RepoID:        repoID,
		RepoBaseRef:   "main",
		RepoTargetRef: "feature",
		Attempt:       1,
		Status:        store.RunRepoStatusQueued,
		CreatedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	st := &startRunStoreMock{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     domaintypes.NewModID(),
			SpecID:    specID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		getSpecResult:                 store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
		listRunReposByRunResult:       []store.RunRepo{queuedRepo},
		listQueuedRunReposByRunResult: []store.RunRepo{queuedRepo},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/start", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	startRunHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.createJobCallCount != 3 {
		t.Fatalf("expected starter to create 3 jobs, got %d", st.createJobCallCount)
	}

	var resp StartRunResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RunID != runID {
		t.Fatalf("expected run id %q, got %q", runID, resp.RunID)
	}
	if resp.Started != 1 || resp.Pending != 1 || resp.AlreadyDone != 0 {
		t.Fatalf("unexpected start response: %+v", resp)
	}
}

func TestStartRunHandler_TerminalRunConflict(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &startRunStoreMock{
		getRunResult: store.Run{
			ID:        runID,
			ModID:     domaintypes.NewModID(),
			SpecID:    domaintypes.NewSpecID(),
			Status:    store.RunStatusCancelled,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/start", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	startRunHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

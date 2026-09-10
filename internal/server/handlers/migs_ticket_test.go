package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
)

const testRepoSHA0 = "0123456789abcdef0123456789abcdef01234567"

type expectedPlannedJob struct {
	name     string
	jobType  domaintypes.JobType
	jobImage string
}

func TestCreateSingleRepoRunHandler_SingleRepo(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := &handlerStore{}
	st.createRun.val = store.Run{
		Status:    domaintypes.RunStatusRunning,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}

	handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{})
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBody())

	assertStatus(t, rr, http.StatusCreated)

	resp := decodeBody[struct {
		RunID  string `json:"run_id"`
		MigID  string `json:"mig_id"`
		SpecID string `json:"spec_id"`
	}](t, rr)

	if resp.RunID == "" {
		t.Fatal("expected run_id to be set")
	}
	if resp.MigID == "" {
		t.Fatal("expected mig_id to be set")
	}
	if resp.SpecID == "" {
		t.Fatal("expected spec_id to be set")
	}

	if !st.createSpec.called || !st.createMig.called || !st.createMigRepo.called || !st.createRun.called {
		t.Fatal("expected spec/mig/repo/run creation calls to be made")
	}
	if len(st.createJob.calls) != 3 {
		t.Fatalf("expected the complete job chain on submission, got %d jobs", len(st.createJob.calls))
	}
}

func TestPlanJobsFromSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		spec     []byte
		expected []expectedPlannedJob
	}{
		{
			name: "SingleMig",
			spec: []byte(`{"steps":[{"image":"mig1:v1"}]}`),
			expected: []expectedPlannedJob{
				{"pre-gate", domaintypes.JobTypePreGate, ""},
				{"mig-0", domaintypes.JobTypeMig, "mig1:v1"},
				{"post-gate", domaintypes.JobTypePostGate, ""},
			},
		},
		{
			name: "MultiStep",
			spec: []byte(`{"steps":[{"image":"mig1:v1"},{"image":"mig2:v2"},{"image":"mig3:v3"}]}`),
			expected: []expectedPlannedJob{
				{"pre-gate", domaintypes.JobTypePreGate, ""},
				{"mig-0", domaintypes.JobTypeMig, "mig1:v1"},
				{"mig-1", domaintypes.JobTypeMig, "mig2:v2"},
				{"mig-2", domaintypes.JobTypeMig, "mig3:v3"},
				{"post-gate", domaintypes.JobTypePostGate, ""},
			},
		},
		{
			name: "BuildGateDisabled",
			spec: []byte(`{"steps":[{"image":"a"},{"image":"b"}],"build_gate":{"disabled":true}}`),
			expected: []expectedPlannedJob{
				{"mig-0", domaintypes.JobTypeMig, "a"},
				{"mig-1", domaintypes.JobTypeMig, "b"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planned, err := planJobsFromSpec(tt.spec)
			if err != nil {
				t.Fatalf("planJobsFromSpec failed: %v", err)
			}
			if len(planned) != len(tt.expected) {
				t.Fatalf("planned jobs=%d, want %d", len(planned), len(tt.expected))
			}
			for i, want := range tt.expected {
				got := planned[i]
				if got.Name != want.name || got.JobType != want.jobType || got.JobImage != want.jobImage {
					t.Fatalf("planned job %d=%+v, want name=%q type=%q image=%q", i, got, want.name, want.jobType, want.jobImage)
				}
			}
		})
	}
}

func TestCreateSingleRepoRunHandler_ValidationErrors(t *testing.T) {
	t.Parallel()

	st := &handlerStore{}
	handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{})

	tests := []struct {
		name       string
		body       any
		wantSubstr string
	}{
		{"empty repo_url", validRunRequestBodyWith(map[string]any{"repo_url": ""}), "empty"},
		{"no repo_url", validRunRequestBodyWithout("repo_url"), "empty"},
		{"empty ref", validRunRequestBodyWith(map[string]any{"ref": ""}), "empty"},
		{"no ref", validRunRequestBodyWithout("ref"), "empty"},
		{"no spec", validRunRequestBodyWithout("spec"), "exactly one of spec or spec_selector is required"},
		{"invalid JSON", "not json", "invalid request"},
		{"http scheme repo_url", validRunRequestBodyWith(map[string]any{"repo_url": "http://github.com/user/repo.git"}), "invalid repo url"},
		{"git scheme repo_url", validRunRequestBodyWith(map[string]any{"repo_url": "git://github.com/user/repo.git"}), "invalid repo url"},
		{"no scheme repo_url", validRunRequestBodyWith(map[string]any{"repo_url": "github.com/user/repo.git"}), "invalid repo url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := doRequest(t, handler, http.MethodPost, "/v1/runs", tt.body)
			assertStatus(t, rr, http.StatusBadRequest)
			assertBodyContains(t, rr, tt.wantSubstr)
		})
	}
}

func TestCreateSingleRepoRunHandler_PublishesEvent(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := &handlerStore{}
	st.createRun.val = store.Run{
		Status:    domaintypes.RunStatusRunning,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}

	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService, gitauth.Options{}, runSubmitSpecServices{})

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBody())
	assertStatus(t, rr, http.StatusCreated)

	resp := decodeBody[struct {
		RunID string `json:"run_id"`
	}](t, rr)
	runID := resp.RunID

	snapshot := eventsService.Hub().Snapshot(domaintypes.RunID(runID))
	if len(snapshot) == 0 {
		t.Fatal("expected at least one run event to be published")
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
		t.Fatal("expected to find a 'run' event in the snapshot")
	}
}

func TestGetRunStatusHandler(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	jobID := domaintypes.NewJobID()
	jobIDStr := jobID.String()
	nextJobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	tests := []struct {
		name       string
		setupStore func() *handlerStore
		reqRunID   string
		wantStatus int
		verify     func(t *testing.T, st *handlerStore, rr *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			setupStore: func() *handlerStore {
				st := &handlerStore{}
				st.listJobsByRun.val = []store.Job{
					{ID: jobID, RunID: runID, Status: domaintypes.JobStatusQueued, NextID: &nextJobID, Meta: withNextIDMeta([]byte(`{}`), float64(1000))},
				}
				st.getRun.val = store.Run{
					ID:          runID,
					RepoID:      "repo_123",
					RepoBaseRef: "main",
					Status:      domaintypes.RunStatusRunning,
					CreatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
				}
				st.createMigRepo.val = store.MigRepo{
					RepoID: "repo_123",
				}
				st.getRun.val.RepoID = "repo_123"
				return st
			},
			reqRunID:   runIDStr,
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *handlerStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				resp := decodeBody[migsapi.RunSummary](t, rr)
				if resp.RunID.String() != runIDStr {
					t.Fatalf("expected run_id %s, got %s", runIDStr, resp.RunID.String())
				}
				if resp.State != migsapi.RunStateRunning {
					t.Fatalf("expected status running, got %s", resp.State)
				}
				if resp.Repository != "https://github.com/user/repo.git" {
					t.Fatalf("expected repo_url https://github.com/user/repo.git, got %s", resp.Repository)
				}
				if resp.Metadata["repo_base_ref"] != "main" {
					t.Fatalf("expected base_ref main, got %s", resp.Metadata["repo_base_ref"])
				}
				if len(resp.Stages) != 1 {
					t.Fatalf("expected 1 stage, got %d", len(resp.Stages))
				}
				if got := resp.Stages[domaintypes.JobID(jobIDStr)].State; got != migsapi.StageStatePending {
					t.Fatalf("expected stage to be pending, got %s", got)
				}
				if got := resp.Stages[domaintypes.JobID(jobIDStr)].NextID; got == nil || *got != nextJobID {
					t.Fatalf("expected stage next_id %s, got %v", nextJobID, got)
				}
				assertCalled(t, "GetRun", st.getRun.called)
				assertCalled(t, "ListJobsByRun", st.listJobsByRun.called)
			},
		},
		{
			name: "not found",
			setupStore: func() *handlerStore {
				st := &handlerStore{}
				st.getRun.err = pgx.ErrNoRows
				return st
			},
			reqRunID:   domaintypes.NewRunID().String(),
			wantStatus: http.StatusNotFound,
			verify: func(t *testing.T, _ *handlerStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertBodyContains(t, rr, "not found")
			},
		},
		{
			name: "empty ID",
			setupStore: func() *handlerStore {
				return &handlerStore{}
			},
			reqRunID:   "",
			wantStatus: http.StatusBadRequest,
			verify: func(t *testing.T, _ *handlerStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertBodyContains(t, rr, "path parameter is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := tt.setupStore()
			handler := getRunStatusHandler(st)
			path := "/v1/runs/" + tt.reqRunID + "/status"
			rr := doRequest(t, handler, http.MethodGet, path, nil, "run_id", tt.reqRunID)
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, st, rr)
			}
		})
	}
}

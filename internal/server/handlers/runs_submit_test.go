package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/gitlabtoken"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/gitlabtokens"
	"github.com/iw2rmb/ploy/internal/store"
)

func asJSONBytes(t *testing.T, v any) []byte {
	t.Helper()
	switch raw := v.(type) {
	case nil:
		return nil
	case []byte:
		return raw
	case string:
		return []byte(raw)
	default:
		b, err := json.Marshal(raw)
		if err != nil {
			t.Fatalf("marshal JSON value: %v", err)
		}
		return b
	}
}

func migWaveTokenStore(secondRepoURL string) *handlerStore {
	st := activeMigWithSpec(domaintypes.NewSpecID())
	st.listMigReposByMig.val = []store.MigRepo{
		{ID: "migRepo1", MigID: "mig123", RepoID: "repo1", BaseRef: "main"},
		{ID: "migRepo2", MigID: "mig123", RepoID: "repo2", BaseRef: "main"},
	}
	st.repoByID = map[domaintypes.RepoID]store.Repo{
		"repo1": {ID: "repo1", Url: "https://gitlab.example.com/org/repo1"},
		"repo2": {ID: "repo2", Url: secondRepoURL},
	}
	return st
}

// =============================================================================
// POST /v1/runs — Create Single-Repo Run (v1 API)
// =============================================================================

// TestRunsCreateSingleRepo_Success verifies POST /v1/runs creates a run with mig side-effect.
// Tests single-repo run creation with automatic mig project creation.
// Contract:
//   - Creates a mig project (mig name == mig id).
//   - Creates a spec row and sets migs.spec_id.
//   - Creates a mig repo row for the provided repo_url.
//   - Creates a wave, one running run row, and its complete job chain.
//   - Response includes wave_id, run_id, mig_id, spec_id.
func TestRunsCreateSingleRepo_Success(t *testing.T) {
	st := &handlerStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService, gitauth.Options{}, runSubmitSpecServices{})

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBody())

	assertStatus(t, rr, http.StatusCreated)

	// Verify store methods were called in order.
	if !st.createSpec.called {
		t.Error("store.CreateSpec was not called")
	}
	if !st.createMig.called {
		t.Error("store.CreateMig was not called")
	}
	if !st.createMigRepo.called {
		t.Error("store.CreateMigRepo was not called")
	}
	if !st.createRun.called {
		t.Error("store.CreateRun was not called")
	}
	if len(st.createJob.calls) != 3 {
		t.Errorf("store.CreateJob calls = %d, want 3", len(st.createJob.calls))
	}

	// Verify mig name == mig id (v1 contract).
	if st.createMig.params.Name != st.createMig.params.ID.String() {
		t.Errorf("mig name (%q) != mig id (%q); v1 requires name == id for single-repo runs",
			st.createMig.params.Name, st.createMig.params.ID.String())
	}

	// Verify spec_id was linked to mig.
	if st.createMig.params.SpecID == nil {
		t.Error("mig was not linked to spec (spec_id is nil)")
	}

	// Verify response shape matches v1 contract.
	var resp struct {
		WaveID string `json:"wave_id"`
		RunID  string `json:"run_id"`
		MigID  string `json:"mig_id"`
		SpecID string `json:"spec_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.WaveID == "" {
		t.Error("response wave_id is empty")
	}
	if resp.RunID == "" {
		t.Error("response run_id is empty")
	}
	if resp.MigID == "" {
		t.Error("response mig_id is empty")
	}
	if resp.SpecID == "" {
		t.Error("response spec_id is empty")
	}
}

// TestRunsCreateSingleRepo_MaterializesJobsImmediately verifies submission includes its complete job chain.
func TestRunsCreateSingleRepo_MaterializesJobsImmediately(t *testing.T) {
	st := &handlerStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService, gitauth.Options{}, runSubmitSpecServices{})

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBody())
	assertStatus(t, rr, http.StatusCreated)

	if len(st.createJob.calls) != 3 {
		t.Fatalf("expected 3 jobs to be created during submission, got %d", len(st.createJob.calls))
	}
}

// TestRunsCreateSingleRepo_RepoURLNormalized verifies repo URLs are normalized.
// Uses types.NormalizeRepoURL for URL normalization.
func TestRunsCreateSingleRepo_RepoURLNormalized(t *testing.T) {
	st := &handlerStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService, gitauth.Options{}, runSubmitSpecServices{})

	// URL with trailing slash and .git suffix — should be normalized.
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
		"repo_url": "https://github.com/org/repo.git/",
	}))
	assertStatus(t, rr, http.StatusCreated)

	// Verify mig_repo was created with normalized URL.
	if !st.createMigRepo.called {
		t.Fatal("store.CreateMigRepo was not called")
	}
	// types.NormalizeRepoURL trims trailing "/" and ".git".
	expectedURL := "https://github.com/org/repo"
	if st.createMigRepo.params.Url != expectedURL {
		t.Errorf("mig_repo URL = %q, want %q (normalized)", st.createMigRepo.params.Url, expectedURL)
	}
}

func TestRunsCreateSingleRepo_SSHRepoURLAcceptedWithoutGitLabToken(t *testing.T) {
	st := &handlerStore{}
	handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{})

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
		"repo_url": "ssh://git@gitlab.example.com/org/repo.git",
	}))

	assertStatus(t, rr, http.StatusCreated)
	if st.createMigRepo.params.Url != "ssh://gitlab.example.com/org/repo" {
		t.Fatalf("mig_repo URL = %q, want normalized ssh URL", st.createMigRepo.params.Url)
	}
}

func TestSubmitGitLabTokenBehavior(t *testing.T) {
	token := "glpat-server-secret"
	hash := gitlabtoken.Hash(token)

	tests := []struct {
		name              string
		newHandler        func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc
		path              string
		body              func() any
		wantRuns          int
		wantStatus        int
		wantRegistryAfter bool
	}{
		{
			name: "single run computes marker and registers token",
			newHandler: func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc {
				return createSingleRepoRunHandler(st, nil, gitauth.Options{GitLabDomain: "gitlab.example.com"}, runSubmitSpecServices{}, registry)
			},
			path: "/v1/runs",
			body: func() any {
				return validRunRequestBodyWith(map[string]any{
					"repo_url":     "https://gitlab.example.com/org/repo",
					"gitlab_token": token,
				})
			},
			wantRuns:          1,
			wantStatus:        http.StatusCreated,
			wantRegistryAfter: true,
		},
		{
			name: "single run releases pre-registered token when create fails",
			newHandler: func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc {
				st.createWaveWithRuns.err = errors.New("database connection failed")
				return createSingleRepoRunHandler(st, nil, gitauth.Options{GitLabDomain: "gitlab.example.com"}, runSubmitSpecServices{}, registry)
			},
			path: "/v1/runs",
			body: func() any {
				return validRunRequestBodyWith(map[string]any{
					"repo_url":     "https://gitlab.example.com/org/repo",
					"gitlab_token": token,
				})
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "mig wave computes same marker for every run",
			newHandler: func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc {
				*st = *migWaveTokenStore("https://gitlab.example.com/org/repo2")
				return createMigRunHandler(st, gitauth.Options{GitLabDomain: "gitlab.example.com"}, registry)
			},
			path: "/v1/migs/mig123/waves",
			body: func() any {
				body := allReposSelector()
				body["gitlab_token"] = token
				return body
			},
			wantRuns:          2,
			wantStatus:        http.StatusCreated,
			wantRegistryAfter: true,
		},
		{
			name: "mig wave releases pre-registered token when create fails",
			newHandler: func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc {
				*st = *migWaveTokenStore("https://gitlab.example.com/org/repo2")
				st.createWaveWithRuns.err = errors.New("database connection failed")
				return createMigRunHandler(st, gitauth.Options{GitLabDomain: "gitlab.example.com"}, registry)
			},
			path: "/v1/migs/mig123/waves",
			body: func() any {
				body := allReposSelector()
				body["gitlab_token"] = token
				return body
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &handlerStore{}
			registry := gitlabtokens.NewRegistry()
			handler := tt.newHandler(st, registry)
			observedDuringCreate := false
			st.createWaveWithRunsHook = func(params store.CreateWaveWithRunsParams) {
				if len(params.Runs) == 0 {
					t.Fatalf("CreateWaveWithRuns called without runs")
				}
				gotToken, ok := registry.Token(hash)
				if !ok || gotToken != token {
					t.Fatalf("registry token during CreateWaveWithRuns = %q, %v; want token", gotToken, ok)
				}
				observedDuringCreate = true
			}
			rr := doRequest(t, handler, http.MethodPost, tt.path, tt.body(), "mig_id", "mig123")
			assertStatus(t, rr, tt.wantStatus)
			if !observedDuringCreate {
				t.Fatalf("expected CreateWaveWithRuns registry observation")
			}
			if len(st.createRunParams) != tt.wantRuns {
				t.Fatalf("CreateRun calls = %d, want %d", len(st.createRunParams), tt.wantRuns)
			}
			for _, params := range st.createRunParams {
				stats := asJSONBytes(t, params.Stats)
				if got := gitlabtoken.HashFromRunStats(stats); got != hash {
					t.Fatalf("run %s token hash marker = %q, want %q", params.ID, got, hash)
				}
				if strings.Contains(string(stats), token) {
					t.Fatalf("run %s stats leaked token", params.ID)
				}
			}
			gotToken, ok := registry.Token(hash)
			if ok != tt.wantRegistryAfter {
				t.Fatalf("registry token present after submit = %v, want %v", ok, tt.wantRegistryAfter)
			}
			if tt.wantRegistryAfter && gotToken != token {
				t.Fatalf("registry token after submit = %q, want token", gotToken)
			}
		})
	}
}

func TestSubmitGitLabTokenDomainValidation(t *testing.T) {
	token := "glpat-server-secret"
	hash := gitlabtoken.Hash(token)

	tests := []struct {
		name       string
		newHandler func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc
		path       string
		body       any
		wantError  string
	}{
		{
			name: "single run rejects token without configured GitLab domain",
			newHandler: func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc {
				return createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{}, registry)
			},
			path: "/v1/runs",
			body: validRunRequestBodyWith(map[string]any{
				"repo_url":     "https://gitlab.example.com/org/repo",
				"gitlab_token": token,
			}),
			wantError: "ephemeral GitLab token requires a configured GitLab domain",
		},
		{
			name: "single run rejects token for different repo host",
			newHandler: func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc {
				return createSingleRepoRunHandler(st, nil, gitauth.Options{GitLabDomain: "gitlab.example.com"}, runSubmitSpecServices{}, registry)
			},
			path: "/v1/runs",
			body: validRunRequestBodyWith(map[string]any{
				"repo_url":     "https://github.com/org/repo",
				"gitlab_token": token,
			}),
			wantError: "ephemeral GitLab token is only allowed for repos on configured GitLab domain gitlab.example.com, got github.com",
		},
		{
			name: "single run rejects token for ssh repo on configured GitLab domain",
			newHandler: func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc {
				return createSingleRepoRunHandler(st, nil, gitauth.Options{GitLabDomain: "gitlab.example.com"}, runSubmitSpecServices{}, registry)
			},
			path: "/v1/runs",
			body: validRunRequestBodyWith(map[string]any{
				"repo_url":     "ssh://git@gitlab.example.com/org/repo",
				"gitlab_token": token,
			}),
			wantError: "ephemeral GitLab token is only allowed for https repos on configured GitLab domain gitlab.example.com, got ssh://gitlab.example.com",
		},
		{
			name: "single run rejects token for file repo",
			newHandler: func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc {
				return createSingleRepoRunHandler(st, nil, gitauth.Options{GitLabDomain: "gitlab.example.com"}, runSubmitSpecServices{}, registry)
			},
			path: "/v1/runs",
			body: validRunRequestBodyWith(map[string]any{
				"repo_url":     "file:///tmp/repo",
				"gitlab_token": token,
			}),
			wantError: "ephemeral GitLab token is only allowed for https repos on configured GitLab domain gitlab.example.com, got file://",
		},
		{
			name: "mig wave rejects token when any selected repo is on another host",
			newHandler: func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc {
				*st = *migWaveTokenStore("https://github.com/org/repo2")
				return createMigRunHandler(st, gitauth.Options{GitLabDomain: "gitlab.example.com"}, registry)
			},
			path: "/v1/migs/mig123/waves",
			body: func() any {
				body := allReposSelector()
				body["gitlab_token"] = token
				return body
			}(),
			wantError: "ephemeral GitLab token is only allowed for repos on configured GitLab domain gitlab.example.com, got github.com",
		},
		{
			name: "mig wave rejects token when selected repo uses ssh",
			newHandler: func(st *handlerStore, registry *gitlabtokens.Registry) http.HandlerFunc {
				*st = *migWaveTokenStore("ssh://git@gitlab.example.com/org/repo2")
				return createMigRunHandler(st, gitauth.Options{GitLabDomain: "gitlab.example.com"}, registry)
			},
			path: "/v1/migs/mig123/waves",
			body: func() any {
				body := allReposSelector()
				body["gitlab_token"] = token
				return body
			}(),
			wantError: "ephemeral GitLab token is only allowed for https repos on configured GitLab domain gitlab.example.com, got ssh://gitlab.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &handlerStore{}
			registry := gitlabtokens.NewRegistry()
			handler := tt.newHandler(st, registry)

			rr := doRequest(t, handler, http.MethodPost, tt.path, tt.body, "mig_id", "mig123")

			assertStatus(t, rr, http.StatusBadRequest)
			if !strings.Contains(rr.Body.String(), tt.wantError) {
				t.Fatalf("response body = %q, want containing %q", rr.Body.String(), tt.wantError)
			}
			if st.createWaveWithRuns.called {
				t.Fatalf("CreateWaveWithRuns should not be called")
			}
			if _, ok := registry.Token(hash); ok {
				t.Fatalf("registry token should not be registered")
			}
		})
	}
}

// TestRunsCreateSingleRepo_ValidationErrors merges individual validation error tests.
func TestRunsCreateSingleRepo_ValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name:       "MissingRepoURL",
			body:       validRunRequestBodyWithout("repo_url"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "InvalidRepoURLScheme",
			body:       validRunRequestBodyWith(map[string]any{"repo_url": "ftp://example.com/repo"}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "MissingRef",
			body:       validRunRequestBodyWithout("ref"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "MissingSpec",
			body:       validRunRequestBodyWithout("spec"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "SpecAndSelector",
			body: validRunRequestBodyWith(map[string]any{
				"spec_selector": "upgrade-java",
			}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "LocalSpecWithNamedOverrides",
			body: validRunRequestBodyWith(map[string]any{
				"spec_overrides": map[string]any{"step_envs": map[string]any{"rewrite": []string{"A=1"}}},
			}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "InvalidSpec",
			body:       validRunRequestBodyWith(map[string]any{"spec": map[string]any{"steps": "not-array"}}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "InvalidJSON",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &handlerStore{}
			handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{})
			rr := doRequest(t, handler, http.MethodPost, "/v1/runs", tt.body)
			assertStatus(t, rr, tt.wantStatus)
		})
	}
}

func TestRunsCreateSingleRepoCreatedByResolution(t *testing.T) {
	tests := []struct {
		name           string
		requestCreated string
		identity       auth.Identity
		tokenUsername  *string
		wantCreatedBy  string
		wantTokenQuery bool
	}{
		{
			name:           "request created_by propagates",
			requestCreated: "test-user@example.com",
			wantCreatedBy:  "test-user@example.com",
		},
		{
			name:           "token username wins over request created_by",
			requestCreated: "request-user",
			identity:       auth.Identity{Role: auth.RoleControlPlane, TokenID: "token-1"},
			tokenUsername:  stringPtrOrNil("token-user"),
			wantCreatedBy:  "token-user",
			wantTokenQuery: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &handlerStore{
				getAPITokenByID: mockCall[string, store.GetAPITokenByIDRow]{
					val: store.GetAPITokenByIDRow{Username: tt.tokenUsername},
				},
			}
			handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{})
			body, _ := json.Marshal(validRunRequestBodyWith(map[string]any{"created_by": tt.requestCreated}))
			req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tt.identity.TokenID != "" {
				req = req.WithContext(auth.ContextWithIdentity(req.Context(), tt.identity))
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assertStatus(t, rr, http.StatusCreated)
			if st.getAPITokenByID.called != tt.wantTokenQuery {
				t.Fatalf("GetAPITokenByID called = %v, want %v", st.getAPITokenByID.called, tt.wantTokenQuery)
			}
			if tt.wantTokenQuery && st.getAPITokenByID.params != tt.identity.TokenID {
				t.Fatalf("GetAPITokenByID token = %q, want %q", st.getAPITokenByID.params, tt.identity.TokenID)
			}
			if st.createSpec.params.CreatedBy == nil || *st.createSpec.params.CreatedBy != tt.wantCreatedBy {
				t.Fatalf("spec created_by = %v, want %q", st.createSpec.params.CreatedBy, tt.wantCreatedBy)
			}
			if st.createMig.params.CreatedBy == nil || *st.createMig.params.CreatedBy != tt.wantCreatedBy {
				t.Fatalf("mig created_by = %v, want %q", st.createMig.params.CreatedBy, tt.wantCreatedBy)
			}
			if st.createWaveWithRuns.params.Wave.CreatedBy == nil || *st.createWaveWithRuns.params.Wave.CreatedBy != tt.wantCreatedBy {
				t.Fatalf("run created_by = %v, want %q", st.createWaveWithRuns.params.Wave.CreatedBy, tt.wantCreatedBy)
			}
		})
	}
}

func TestRunsCreateSingleRepo_RejectsLegacySpecID(t *testing.T) {
	st := &handlerStore{}
	handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{})

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
		"spec_id": "spec1234",
	}))

	assertStatus(t, rr, http.StatusBadRequest)
	assertBodyContains(t, rr, `unknown field "spec_id"`)
	if st.createSpec.called {
		t.Fatal("CreateSpec should not be called for rejected spec_id submissions")
	}
}

// TestRunsCreateSingleRepo_MultiStepSpec verifies POST /v1/runs materializes every spec step.
func TestRunsCreateSingleRepo_MultiStepSpec(t *testing.T) {
	st := &handlerStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService, gitauth.Options{}, runSubmitSpecServices{})

	// Multi-step spec with steps[] array.
	multiStepSpec := map[string]any{
		"envs": map[string]any{},
		"steps": []any{
			map[string]any{"image": "mig-image-1"},
			map[string]any{"image": "mig-image-2"},
		},
	}
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
		"spec": multiStepSpec,
	}))
	assertStatus(t, rr, http.StatusCreated)

	if len(st.createJob.calls) != 4 {
		t.Errorf("createJobCallCount = %d, want 4", len(st.createJob.calls))
	}
}

// =============================================================================
// Store Error Tests (table-driven)
// =============================================================================

// TestRunsCreateSingleRepo_StoreErrors merges individual store error tests.
func TestRunsCreateSingleRepo_StoreErrors(t *testing.T) {
	tests := []struct {
		name    string
		setupFn func(st *handlerStore)
	}{
		{
			name:    "CreateSpecError",
			setupFn: func(st *handlerStore) { st.createSpec.err = errors.New("database connection failed") },
		},
		{
			name:    "CreateMigError",
			setupFn: func(st *handlerStore) { st.createMig.err = errors.New("database connection failed") },
		},
		{
			name:    "CreateMigRepoError",
			setupFn: func(st *handlerStore) { st.createMigRepo.err = errors.New("database connection failed") },
		},
		{
			name:    "CreateRunError",
			setupFn: func(st *handlerStore) { st.createRunSeq.errs = []error{errors.New("database connection failed")} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &handlerStore{}
			tt.setupFn(st)
			handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{})
			rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBody())
			assertStatus(t, rr, http.StatusInternalServerError)
		})
	}
}

func TestRunsCreateSingleRepo_RejectsWhenSourceCommitSeedFails(t *testing.T) {
	st := &handlerStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService, gitauth.Options{}, runSubmitSpecServices{})

	body, _ := json.Marshal(validRunRequestBody())
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withSourceCommitSHAResolver(req.Context(), func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("seed lookup failed")
	}))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	if st.createSpec.called {
		t.Fatal("store.CreateSpec should not be called when source commit seed resolution fails")
	}
	if st.createMig.called {
		t.Fatal("store.CreateMig should not be called when source commit seed resolution fails")
	}
	if st.createMigRepo.called {
		t.Fatal("store.CreateMigRepo should not be called when source commit seed resolution fails")
	}
	if st.createRun.called {
		t.Fatal("store.CreateRun should not be called when source commit seed resolution fails")
	}
	if st.createRun.called {
		t.Fatal("store.CreateRun should not be called when source commit seed resolution fails")
	}
}

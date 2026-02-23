package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestClaimJob_Success(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "mod-0",
			Status:      store.JobStatusRunning,
			ModType:     domaintypes.ModTypeMod.String(),
			StepIndex:   domaintypes.StepIndex(2000),
			Meta:        []byte(`{}`),
		},
		getRunResult: store.Run{
			ID:        runID,
			SpecID:    specID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        store.RunRepoStatusQueued,
			Attempt:       1,
		},
		getModRepoResult: store.ModRepo{
			ID:      repoID,
			RepoUrl: "https://github.com/user/repo.git",
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
	}

	handler := claimJobHandler(st, &ConfigHolder{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeKey+"/claim", nil)
	req.SetPathValue("id", nodeKey)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.claimJobCalled || string(st.claimJobParams) != nodeID.String() {
		t.Fatalf("expected ClaimJob to be called with node id")
	}
	if len(st.updateRunRepoStatusParams) == 0 {
		t.Fatalf("expected UpdateRunRepoStatus to be called")
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["id"] != runID.String() {
		t.Fatalf("expected id (run_id) %s, got %v", runID.String(), resp["id"])
	}
	if resp["job_id"] != jobID.String() {
		t.Fatalf("expected job_id %s, got %v", jobID.String(), resp["job_id"])
	}
	if resp["repo_id"] != repoID.String() {
		t.Fatalf("expected repo_id %s, got %v", repoID.String(), resp["repo_id"])
	}
	if resp["repo_url"] != "https://github.com/user/repo.git" {
		t.Fatalf("expected repo_url, got %v", resp["repo_url"])
	}
	if resp["base_ref"] != "main" {
		t.Fatalf("expected base_ref main, got %v", resp["base_ref"])
	}
	if resp["target_ref"] != "feature-branch" {
		t.Fatalf("expected target_ref feature-branch, got %v", resp["target_ref"])
	}
	// v1: run status should be "Started" (not HEAD literals like "assigned"/"running").
	// v1 run status values are Started, Cancelled, or Finished.
	if resp["status"] != "Started" {
		t.Fatalf("expected status Started, got %v", resp["status"])
	}

	spec, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec to be an object, got %T", resp["spec"])
	}
	if spec["job_id"] != jobID.String() {
		t.Fatalf("expected spec.job_id %s, got %v", jobID.String(), spec["job_id"])
	}
	if _, ok := spec["mod_index"]; ok {
		t.Fatalf("expected spec.mod_index to be absent, got %v", spec["mod_index"])
	}
}

func TestClaimJob_SpecFromDBMustBeJSONObject(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "mod-0",
			Status:      store.JobStatusRunning,
			ModType:     domaintypes.ModTypeMod.String(),
			StepIndex:   domaintypes.StepIndex(2000),
			Meta:        []byte(`{}`),
		},
		getRunResult: store.Run{
			ID:        runID,
			SpecID:    specID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        store.RunRepoStatusQueued,
			Attempt:       1,
		},
		getModRepoResult: store.ModRepo{
			ID:      repoID,
			RepoUrl: "https://github.com/user/repo.git",
		},
		// Spec is sourced from the DB at claim time; it must be a JSON object.
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`[]`)},
	}

	handler := claimJobHandler(st, &ConfigHolder{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeKey+"/claim", nil)
	req.SetPathValue("id", nodeKey)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestClaimJob_MRJob_DoesNotUpdateRunRepoStatus(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "mr-0",
			Status:      store.JobStatusRunning,
			ModType:     domaintypes.ModTypeMR.String(),
			StepIndex:   domaintypes.StepIndex(2000),
			Meta:        []byte(`{}`),
		},
		getRunResult: store.Run{
			ID:        runID,
			SpecID:    specID,
			Status:    store.RunStatusFinished,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        store.RunRepoStatusSuccess,
			Attempt:       1,
		},
		getModRepoResult: store.ModRepo{
			ID:      repoID,
			RepoUrl: "https://github.com/user/repo.git",
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
	}

	handler := claimJobHandler(st, &ConfigHolder{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeKey+"/claim", nil)
	req.SetPathValue("id", nodeKey)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(st.updateRunRepoStatusParams) != 0 {
		t.Fatalf("expected UpdateRunRepoStatus not to be called for MR jobs")
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "Finished" {
		t.Fatalf("expected status Finished, got %v", resp["status"])
	}
	if resp["repo_url"] != "https://github.com/user/repo.git" {
		t.Fatalf("expected repo_url, got %v", resp["repo_url"])
	}
	if resp["base_ref"] != "main" {
		t.Fatalf("expected base_ref main, got %v", resp["base_ref"])
	}
	if resp["target_ref"] != "feature-branch" {
		t.Fatalf("expected target_ref feature-branch, got %v", resp["target_ref"])
	}
}

func TestClaimJob_NoJobsAvailable(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		// claimJobResult left empty: mock ClaimJob returns pgx.ErrNoRows.
	}

	handler := claimJobHandler(st, &ConfigHolder{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rr.Code)
	}
}

func TestClaimJob_NodeNotFound(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	st := &mockStore{getNodeErr: pgx.ErrNoRows}

	handler := claimJobHandler(st, &ConfigHolder{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID+"/claim", nil)
	req.SetPathValue("id", nodeID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

func TestClaimJob_MergesGlobalEnvIntoSpec(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	runSpec := []byte(`{"env":{"CA_CERTS_PEM_BUNDLE":"per-run-cert","PER_RUN_ONLY":"value"}}`)

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "mod-0",
			Status:      store.JobStatusRunning,
			ModType:     domaintypes.ModTypeMod.String(),
			StepIndex:   domaintypes.StepIndex(2000),
			Meta:        []byte(`{}`),
		},
		getRunResult: store.Run{
			ID:        runID,
			SpecID:    specID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        store.RunRepoStatusQueued,
			Attempt:       1,
		},
		getModRepoResult: store.ModRepo{
			ID:      repoID,
			RepoUrl: "https://github.com/user/repo.git",
		},
		getSpecResult: store.Spec{ID: specID, Spec: runSpec},
	}

	configHolder := &ConfigHolder{}
	configHolder.SetGlobalEnvVar("CA_CERTS_PEM_BUNDLE", GlobalEnvVar{Value: "global-cert", Scope: domaintypes.GlobalEnvScopeAll, Secret: true})
	configHolder.SetGlobalEnvVar("CODEX_AUTH_JSON", GlobalEnvVar{Value: `{"token":"xxx"}`, Scope: domaintypes.GlobalEnvScopeMods, Secret: true})
	configHolder.SetGlobalEnvVar("HEAL_ONLY", GlobalEnvVar{Value: "heal-env", Scope: domaintypes.GlobalEnvScopeHeal, Secret: false})

	handler := claimJobHandler(st, configHolder, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeKey+"/claim", nil)
	req.SetPathValue("id", nodeKey)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	spec, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec to be an object, got %T", resp["spec"])
	}
	env, ok := spec["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec.env to be an object, got %T", spec["env"])
	}

	if env["CA_CERTS_PEM_BUNDLE"] != "per-run-cert" {
		t.Fatalf("expected per-run CA_CERTS_PEM_BUNDLE to win, got %v", env["CA_CERTS_PEM_BUNDLE"])
	}
	if env["CODEX_AUTH_JSON"] != `{"token":"xxx"}` {
		t.Fatalf("expected CODEX_AUTH_JSON to be injected, got %v", env["CODEX_AUTH_JSON"])
	}
	if _, ok := env["HEAL_ONLY"]; ok {
		t.Fatalf("expected HEAL_ONLY not to be injected for mod job")
	}
	if env["PER_RUN_ONLY"] != "value" {
		t.Fatalf("expected PER_RUN_ONLY preserved, got %v", env["PER_RUN_ONLY"])
	}
}

func TestClaimJob_ResponseUsesNextIDContract(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "mod-0",
			Status:      store.JobStatusRunning,
			ModType:     domaintypes.ModTypeMod.String(),
			StepIndex:   domaintypes.StepIndex(2000),
			Meta:        []byte(`{}`),
		},
		getRunResult: store.Run{
			ID:        runID,
			SpecID:    specID,
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        store.RunRepoStatusQueued,
			Attempt:       1,
		},
		getModRepoResult: store.ModRepo{
			ID:      repoID,
			RepoUrl: "https://github.com/user/repo.git",
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
	}

	handler := claimJobHandler(st, &ConfigHolder{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeKey+"/claim", nil)
	req.SetPathValue("id", nodeKey)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if _, ok := resp["next_id"]; !ok {
		t.Fatalf("expected claim response to include next_id")
	}
	if _, ok := resp["step_index"]; ok {
		t.Fatalf("expected claim response to omit step_index")
	}
}

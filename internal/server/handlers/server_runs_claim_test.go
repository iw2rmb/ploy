package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type panicInIsError struct{}

func (panicInIsError) Error() string { return "panic-in-is" }
func (panicInIsError) Is(error) bool { panic("boom from Is") }

type panicInErrorString struct{}

func (panicInErrorString) Error() string { panic("boom from Error") }

func TestClaimJob_Success(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
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
			Name:        "mig-0",
			Status:      store.JobStatusRunning,
			JobType:     domaintypes.JobTypeMod.String(),
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
		getModRepoResult: store.MigRepo{
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
	if got, ok := resp["repo_gate_profile_missing"].(bool); !ok || !got {
		t.Fatalf("expected repo_gate_profile_missing=true, got %v", resp["repo_gate_profile_missing"])
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
	repoID := domaintypes.NewMigRepoID()
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
			Name:        "mig-0",
			Status:      store.JobStatusRunning,
			JobType:     domaintypes.JobTypeMod.String(),
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
		getModRepoResult: store.MigRepo{
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
	repoID := domaintypes.NewMigRepoID()
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
			JobType:     domaintypes.JobTypeMR.String(),
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
		getModRepoResult: store.MigRepo{
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
	repoID := domaintypes.NewMigRepoID()
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
			Name:        "mig-0",
			Status:      store.JobStatusRunning,
			JobType:     domaintypes.JobTypeMod.String(),
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
		getModRepoResult: store.MigRepo{
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
		t.Fatalf("expected HEAL_ONLY not to be injected for mig job")
	}
	if env["PER_RUN_ONLY"] != "value" {
		t.Fatalf("expected PER_RUN_ONLY preserved, got %v", env["PER_RUN_ONLY"])
	}
}

func TestClaimJob_MergesGateProfileIntoGateSpec(t *testing.T) {
	t.Parallel()

	profile := []byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"runtime": {
			"docker": {
				"mode": "host_socket"
			}
		},
		"targets": {
			"build": {
				"status": "passed",
				"command": "go test ./...",
				"env": {"GOFLAGS":"-mod=readonly"},
				"failure_code": null
			},
			"unit": {
				"status": "passed",
				"command": "go test ./... -run TestUnit",
				"env": {"CGO_ENABLED":"0"},
				"failure_code": null
			},
			"all_tests": {
				"status": "not_attempted",
				"env": {}
			}
		},
		"orchestration": {"pre": [], "post": []}
	}`)

	tests := []struct {
		name      string
		jobType   domaintypes.JobType
		spec      []byte
		wantPhase string
		wantCmd   string
		wantEnvK  string
		wantEnvV  string
	}{
		{
			name:      "pre_gate maps targets.build to build_gate.pre.gate_profile",
			jobType:   domaintypes.JobTypePreGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			wantPhase: "pre",
			wantCmd:   "go test ./...",
			wantEnvK:  "DOCKER_HOST",
			wantEnvV:  "unix:///var/run/docker.sock",
		},
		{
			name:      "post_gate maps targets.unit to build_gate.post.gate_profile",
			jobType:   domaintypes.JobTypePostGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			wantPhase: "post",
			wantCmd:   "go test ./... -run TestUnit",
			wantEnvK:  "CGO_ENABLED",
			wantEnvV:  "0",
		},
		{
			name:      "re_gate reuses post phase mapping from targets.unit",
			jobType:   domaintypes.JobTypeReGate,
			spec:      []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			wantPhase: "post",
			wantCmd:   "go test ./... -run TestUnit",
			wantEnvK:  "CGO_ENABLED",
			wantEnvV:  "0",
		},
		{
			name:    "explicit spec gate_profile wins over profile mapping",
			jobType: domaintypes.JobTypePreGate,
			spec: []byte(`{
				"steps":[{"image":"docker.io/acme/mod:latest"}],
				"build_gate":{"pre":{"gate_profile":{"command":"echo explicit","env":{"X":"1"}}}}
			}`),
			wantPhase: "pre",
			wantCmd:   "echo explicit",
			wantEnvK:  "X",
			wantEnvV:  "1",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			nodeKey := domaintypes.NewNodeKey()
			nodeID := domaintypes.NodeID(nodeKey)
			runID := domaintypes.NewRunID()
			repoID := domaintypes.NewMigRepoID()
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
					Name:        "gate-0",
					Status:      store.JobStatusRunning,
					JobType:     tc.jobType.String(),
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
				getModRepoResult: store.MigRepo{
					ID:          repoID,
					RepoUrl:     "https://github.com/user/repo.git",
					GateProfile: profile,
				},
				getSpecResult: store.Spec{ID: specID, Spec: tc.spec},
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
			if got, ok := resp["repo_gate_profile_missing"].(bool); !ok || got {
				t.Fatalf("expected repo_gate_profile_missing=false, got %v", resp["repo_gate_profile_missing"])
			}
			spec, ok := resp["spec"].(map[string]any)
			if !ok {
				t.Fatalf("expected spec object, got %T", resp["spec"])
			}
			bg, ok := spec["build_gate"].(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate object, got %T", spec["build_gate"])
			}
			phase, ok := bg[tc.wantPhase].(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate.%s object, got %T", tc.wantPhase, bg[tc.wantPhase])
			}
			prep, ok := phase["gate_profile"].(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate.%s.gate_profile object, got %T", tc.wantPhase, phase["gate_profile"])
			}
			if got := prep["command"]; got != tc.wantCmd {
				t.Fatalf("build_gate.%s.gate_profile.command=%v, want %q", tc.wantPhase, got, tc.wantCmd)
			}
			env, ok := prep["env"].(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate.%s.gate_profile.env object, got %T", tc.wantPhase, prep["env"])
			}
			if got := env[tc.wantEnvK]; got != tc.wantEnvV {
				t.Fatalf("build_gate.%s.gate_profile.env[%s]=%v, want %q", tc.wantPhase, tc.wantEnvK, got, tc.wantEnvV)
			}
		})
	}
}

func TestClaimJob_ReGateCandidatePrepOverridePrecedence(t *testing.T) {
	t.Parallel()

	repoProfile := []byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"targets": {
			"build": {"status":"passed","command":"echo repo-build","env":{},"failure_code":null},
			"unit": {"status":"passed","command":"echo repo-unit","env":{"SRC":"repo"},"failure_code":null},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []}
	}`)
	candidateProfile := `{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"targets": {
			"build": {"status":"passed","command":"echo candidate-build","env":{},"failure_code":null},
			"unit": {"status":"passed","command":"echo candidate-unit","env":{"SRC":"candidate"},"failure_code":null},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []}
	}`

	tests := []struct {
		name    string
		spec    []byte
		wantCmd string
		wantSrc string
	}{
		{
			name:    "candidate wins over repo gate_profile on re_gate",
			spec:    []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			wantCmd: "echo candidate-unit",
			wantSrc: "candidate",
		},
		{
			name: "explicit prep wins over candidate and repo",
			spec: []byte(`{
				"steps":[{"image":"docker.io/acme/mod:latest"}],
				"build_gate":{"post":{"gate_profile":{"command":"echo explicit","env":{"SRC":"explicit"}}}}
			}`),
			wantCmd: "echo explicit",
			wantSrc: "explicit",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			nodeKey := domaintypes.NewNodeKey()
			nodeID := domaintypes.NodeID(nodeKey)
			runID := domaintypes.NewRunID()
			repoID := domaintypes.NewMigRepoID()
			specID := domaintypes.NewSpecID()
			jobID := domaintypes.NewJobID()
			now := time.Now().UTC()

			meta := fmt.Sprintf(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"%s","candidate_artifact_path":"%s","candidate_validation_status":"%s","candidate_gate_profile":%s}}`,
				contracts.GateProfileCandidateSchemaID,
				contracts.GateProfileCandidateArtifactPath,
				contracts.RecoveryCandidateStatusValid,
				candidateProfile,
			)
			st := &mockStore{
				getNodeResult: store.Node{ID: nodeID},
				claimJobResult: store.Job{
					ID:          jobID,
					RunID:       runID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     1,
					NodeID:      &nodeID,
					Name:        "re-gate-1",
					Status:      store.JobStatusRunning,
					JobType:     domaintypes.JobTypeReGate.String(),
					Meta:        []byte(meta),
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
				getModRepoResult: store.MigRepo{
					ID:          repoID,
					RepoUrl:     "https://github.com/user/repo.git",
					GateProfile: repoProfile,
				},
				getSpecResult: store.Spec{ID: specID, Spec: tc.spec},
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
			spec, ok := resp["spec"].(map[string]any)
			if !ok {
				t.Fatalf("expected spec object, got %T", resp["spec"])
			}
			bg, ok := spec["build_gate"].(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate object, got %T", spec["build_gate"])
			}
			post, ok := bg["post"].(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate.post object, got %T", bg["post"])
			}
			prep, ok := post["gate_profile"].(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate.post.gate_profile object, got %T", post["gate_profile"])
			}
			if got := prep["command"]; got != tc.wantCmd {
				t.Fatalf("build_gate.post.gate_profile.command=%v, want %q", got, tc.wantCmd)
			}
			env, ok := prep["env"].(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate.post.gate_profile.env object, got %T", prep["env"])
			}
			if got := env["SRC"]; got != tc.wantSrc {
				t.Fatalf("build_gate.post.gate_profile.env[SRC]=%v, want %q", got, tc.wantSrc)
			}
		})
	}
}

func TestClaimJob_InvalidGateProfileReturnsError(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
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
			Name:        "pre-gate-0",
			Status:      store.JobStatusRunning,
			JobType:     domaintypes.JobTypePreGate.String(),
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
		getModRepoResult: store.MigRepo{
			ID:          repoID,
			RepoUrl:     "https://github.com/user/repo.git",
			GateProfile: []byte(`{"schema_version":1}`),
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
	}

	handler := claimJobHandler(st, &ConfigHolder{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeKey+"/claim", nil)
	req.SetPathValue("id", nodeKey)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "gate_profile") {
		t.Fatalf("expected gate_profile error, got %q", rr.Body.String())
	}
}

func TestClaimJob_HealMergesSelectedErrorKindAndExpectedArtifacts(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	spec := []byte(`{
		"steps":[{"image":"docker.io/acme/mod:latest"}],
		"build_gate":{
			"healing":{
				"by_error_kind":{
					"infra":{"retries":2,"image":"docker.io/acme/heal:latest"}
				}
			},
			"router":{"image":"docker.io/acme/router:latest"}
		}
	}`)

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "heal-1-0",
			Status:      store.JobStatusRunning,
			JobType:     domaintypes.JobTypeHeal.String(),
			JobImage:    "docker.io/acme/heal:latest",
			Meta:        []byte(`{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}}`),
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
		getModRepoResult: store.MigRepo{
			ID:      repoID,
			RepoUrl: "https://github.com/user/repo.git",
		},
		getSpecResult: store.Spec{ID: specID, Spec: spec},
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
	specObj, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec object, got %T", resp["spec"])
	}
	bg, ok := specObj["build_gate"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate object, got %T", specObj["build_gate"])
	}
	healing, ok := bg["healing"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate.healing object, got %T", bg["healing"])
	}
	if got := healing["selected_error_kind"]; got != "infra" {
		t.Fatalf("build_gate.healing.selected_error_kind=%v, want infra", got)
	}
	paths, ok := specObj["artifact_paths"].([]any)
	if !ok {
		t.Fatalf("expected artifact_paths array, got %T", specObj["artifact_paths"])
	}
	found := false
	for _, p := range paths {
		if s, ok := p.(string); ok && s == "/out/gate-profile-candidate.json" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected artifact_paths to include /out/gate-profile-candidate.json, got %#v", paths)
	}
	envObj, ok := specObj["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec.env object, got %T", specObj["env"])
	}
	schemaRaw, ok := envObj[contracts.GateProfileSchemaJSONEnv].(string)
	if !ok || strings.TrimSpace(schemaRaw) == "" {
		t.Fatalf("expected %s in spec.env, got %v", contracts.GateProfileSchemaJSONEnv, envObj[contracts.GateProfileSchemaJSONEnv])
	}
	if !json.Valid([]byte(schemaRaw)) {
		t.Fatalf("expected %s to be valid JSON", contracts.GateProfileSchemaJSONEnv)
	}
	rc, ok := resp["recovery_context"].(map[string]any)
	if !ok {
		t.Fatalf("expected recovery_context object, got %T", resp["recovery_context"])
	}
	if got := rc["selected_error_kind"]; got != "infra" {
		t.Fatalf("recovery_context.selected_error_kind=%v, want infra", got)
	}
	if got := rc["resolved_healing_image"]; got != "docker.io/acme/heal:latest" {
		t.Fatalf("recovery_context.resolved_healing_image=%v, want docker.io/acme/heal:latest", got)
	}
	if _, ok := rc["gate_profile_schema_json"].(string); !ok {
		t.Fatalf("expected recovery_context.gate_profile_schema_json string, got %T", rc["gate_profile_schema_json"])
	}
}

func TestClaimJob_HealNonInfraDoesNotInjectSchemaEnv(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	spec := []byte(`{
		"steps":[{"image":"docker.io/acme/mod:latest"}],
		"build_gate":{
			"healing":{
				"by_error_kind":{
					"infra":{"retries":2,"image":"docker.io/acme/heal:latest"},
					"code":{"retries":1,"image":"docker.io/acme/heal:latest"}
				}
			}
		}
	}`)

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "heal-1-0",
			Status:      store.JobStatusRunning,
			JobType:     domaintypes.JobTypeHeal.String(),
			JobImage:    "docker.io/acme/heal:latest",
			Meta:        []byte(`{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"code","strategy_id":"code-default"}}`),
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
		getModRepoResult: store.MigRepo{
			ID:      repoID,
			RepoUrl: "https://github.com/user/repo.git",
		},
		getSpecResult: store.Spec{ID: specID, Spec: spec},
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
	specObj, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec object, got %T", resp["spec"])
	}
	envObj, _ := specObj["env"].(map[string]any)
	if envObj != nil {
		if _, ok := envObj[contracts.GateProfileSchemaJSONEnv]; ok {
			t.Fatalf("did not expect %s for non-infra heal", contracts.GateProfileSchemaJSONEnv)
		}
	}
	rc, ok := resp["recovery_context"].(map[string]any)
	if !ok {
		t.Fatalf("expected recovery_context object, got %T", resp["recovery_context"])
	}
	if got := rc["selected_error_kind"]; got != "code" {
		t.Fatalf("recovery_context.selected_error_kind=%v, want code", got)
	}
	if _, ok := rc["gate_profile_schema_json"]; ok {
		t.Fatalf("did not expect recovery_context.gate_profile_schema_json for non-infra heal")
	}
}

func TestClaimJob_ResponseUsesNextIDContract(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
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
			Name:        "mig-0",
			Status:      store.JobStatusRunning,
			JobType:     domaintypes.JobTypeMod.String(),
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
		getModRepoResult: store.MigRepo{
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
}

func TestClaimJob_ClaimErrorWithPanickingIs_DoesNotPanic(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobErr:   panicInIsError{},
	}

	handler := claimJobHandler(st, &ConfigHolder{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}

func TestClaimJob_ClaimErrorWithPanickingErrorString_DoesNotPanic(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobErr:   panicInErrorString{},
	}

	handler := claimJobHandler(st, &ConfigHolder{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "panic while reading error string") {
		t.Fatalf("expected panic-safe fallback error text, got %q", rr.Body.String())
	}
}

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestClaimJob_ReGateCandidatePrepOverridePrecedence(t *testing.T) {
	t.Parallel()

	candidateProfile := `{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"go","tool":"go"},
		"targets": {
			"active": "unit",
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
			name:    "candidate wins on re_gate",
			spec:    []byte(`{"steps":[{"image":"docker.io/acme/mod:latest"}]}`),
			wantCmd: "echo candidate-unit",
			wantSrc: "candidate",
		},
		{
			name: "explicit prep wins over candidate",
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
			repoID := domaintypes.NewRepoID()
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
					Status:      domaintypes.JobStatusRunning,
					JobType:     domaintypes.JobTypeReGate,
					Meta:        []byte(meta),
				},
				getRunResult: store.Run{
					ID:        runID,
					SpecID:    specID,
					Status:    domaintypes.RunStatusStarted,
					CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
					StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
				},
				getRunRepoResult: store.RunRepo{
					RunID:         runID,
					RepoID:        repoID,
					RepoBaseRef:   "main",
					RepoTargetRef: "feature-branch",
					Status:        domaintypes.RunRepoStatusQueued,
					Attempt:       1,
				},
				getModRepoResult: store.MigRepo{
					ID:     domaintypes.NewMigRepoID(),
					RepoID: repoID,
				},
				getSpecResult: store.Spec{ID: specID, Spec: tc.spec},
			}

			handler := claimJobHandler(st, &ConfigHolder{})
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

func TestClaimJob_InvalidRecoveryCandidateGateProfileReturnsError(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
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
			Name:        "re-gate-0",
			Status:      domaintypes.JobStatusRunning,
			JobType:     domaintypes.JobTypeReGate,
			Meta: []byte(fmt.Sprintf(
				`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"%s","candidate_artifact_path":"%s","candidate_validation_status":"%s","candidate_gate_profile":{"schema_version":1}}}`,
				contracts.GateProfileCandidateSchemaID,
				contracts.GateProfileCandidateArtifactPath,
				contracts.RecoveryCandidateStatusValid,
			)),
		},
		getRunResult: store.Run{
			ID:        runID,
			SpecID:    specID,
			Status:    domaintypes.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
		getModRepoResult: store.MigRepo{
			ID:     domaintypes.NewMigRepoID(),
			RepoID: repoID,
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
	}

	handler := claimJobHandler(st, &ConfigHolder{})
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

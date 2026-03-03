package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type stubGateProfileResolver struct {
	resolution *GateProfileResolution
	err        error
}

func (s *stubGateProfileResolver) ResolveGateProfileForJob(_ context.Context, _ store.Job) (*GateProfileResolution, error) {
	return s.resolution, s.err
}

func TestBuildAndSendJobClaimResponse_ExactHitAddsGateSkipMetadata(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	jobID := domaintypes.NewJobID()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	specID := domaintypes.NewSpecID()
	profileJSON := []byte(`{
		"schema_version": 1,
		"repo_id": "repo_1",
		"runner_mode": "simple",
		"stack": {"language": "java", "tool": "maven", "release": "17"},
		"targets": {
			"active": "unit",
			"build": {"status": "passed", "command": "echo build", "env": {}},
			"unit": {"status": "passed", "command": "echo unit", "env": {}},
			"all_tests": {"status": "not_attempted", "env": {}}
		},
		"orchestration": {"pre": [], "post": []}
	}`)

	job := store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		Name:        "pre-gate",
		JobType:     domaintypes.JobTypePreGate.String(),
		RepoShaIn:   "0123456789abcdef0123456789abcdef01234567",
		NodeID:      &nodeID,
	}
	run := store.Run{
		ID:        runID,
		SpecID:    specID,
		Status:    store.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	runRepo := store.RunRepo{
		RunID:         runID,
		RepoID:        repoID,
		RepoTargetRef: "feature",
	}
	spec := []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}],"build_gate":{"pre":{"target":"unit"}}}`)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/nodes/node_xxx/claim", nil)
	resolver := &stubGateProfileResolver{
		resolution: &GateProfileResolution{
			ProfileID: 77,
			Payload:   profileJSON,
			ExactHit:  true,
		},
	}

	if err := buildAndSendJobClaimResponse(
		rr,
		req,
		&mockStore{},
		&ConfigHolder{},
		run,
		spec,
		runRepo,
		"https://github.com/acme/repo.git",
		job,
		resolver,
	); err != nil {
		t.Fatalf("buildAndSendJobClaimResponse() error = %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	skip, ok := resp["gate_skip"].(map[string]any)
	if !ok {
		t.Fatalf("expected gate_skip object, got %T", resp["gate_skip"])
	}
	if got := skip["enabled"]; got != true {
		t.Fatalf("gate_skip.enabled=%v, want true", got)
	}
	if got := int64(skip["source_profile_id"].(float64)); got != 77 {
		t.Fatalf("gate_skip.source_profile_id=%d, want 77", got)
	}
	if got := skip["matched_target"]; got != contracts.GateProfileTargetUnit {
		t.Fatalf("gate_skip.matched_target=%v, want %q", got, contracts.GateProfileTargetUnit)
	}
}

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

func (s *stubGateProfileResolver) ResolveGateProfileForJob(_ context.Context, _ store.Job, _ GateProfileLookupConstraints) (*GateProfileResolution, error) {
	return s.resolution, s.err
}

func TestBuildAndSendJobClaimResponse_GateSkipScenarios(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	jobID := domaintypes.NewJobID()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	specID := domaintypes.NewSpecID()

	baseJob := store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		Name:        "pre-gate",
		JobType:     domaintypes.JobTypePreGate,
		RepoShaIn:   "0123456789abcdef0123456789abcdef01234567",
		NodeID:      &nodeID,
	}
	baseRun := store.Run{
		ID:        runID,
		SpecID:    specID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	baseRunRepo := store.RunRepo{
		RunID:         runID,
		RepoID:        repoID,
		RepoTargetRef: "feature",
	}

	cases := []struct {
		name          string
		spec          []byte
		profileJSON   []byte
		exactHit      bool
		expectSkip    bool
		expectMatched string
	}{
		{
			name: "exact hit and required target passed -> skip",
			spec: []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}],"build_gate":{"pre":{"target":"build"}}}`),
			profileJSON: gateProfilePayloadForTest(
				contracts.GateProfileTargetBuild,
				contracts.PrepTargetStatusPassed,
				contracts.PrepTargetStatusNotAttempted,
				contracts.PrepTargetStatusNotAttempted,
			),
			exactHit:      true,
			expectSkip:    true,
			expectMatched: contracts.GateProfileTargetBuild,
		},
		{
			name: "exact hit and required target not passed but another passed -> skip by fallback target",
			spec: []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}],"build_gate":{"pre":{"target":"unit"}}}`),
			profileJSON: gateProfilePayloadForTest(
				contracts.GateProfileTargetBuild,
				contracts.PrepTargetStatusPassed,
				contracts.PrepTargetStatusNotAttempted,
				contracts.PrepTargetStatusNotAttempted,
			),
			exactHit:      true,
			expectSkip:    true,
			expectMatched: contracts.GateProfileTargetBuild,
		},
		{
			name: "exact hit but always true -> do not skip",
			spec: []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}],"build_gate":{"pre":{"target":"build","always":true}}}`),
			profileJSON: gateProfilePayloadForTest(
				contracts.GateProfileTargetBuild,
				contracts.PrepTargetStatusPassed,
				contracts.PrepTargetStatusNotAttempted,
				contracts.PrepTargetStatusNotAttempted,
			),
			exactHit:   true,
			expectSkip: false,
		},
		{
			name: "fallback profile (not exact) -> do not skip",
			spec: []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}],"build_gate":{"pre":{"target":"build"}}}`),
			profileJSON: gateProfilePayloadForTest(
				contracts.GateProfileTargetBuild,
				contracts.PrepTargetStatusPassed,
				contracts.PrepTargetStatusNotAttempted,
				contracts.PrepTargetStatusNotAttempted,
			),
			exactHit:   false,
			expectSkip: false,
		},
		{
			name: "exact hit but no passed targets -> do not skip",
			spec: []byte(`{"steps":[{"image":"docker.io/acme/mig:latest"}],"build_gate":{"pre":{"target":"build"}}}`),
			profileJSON: gateProfilePayloadForTest(
				contracts.GateProfileTargetBuild,
				contracts.PrepTargetStatusNotAttempted,
				contracts.PrepTargetStatusNotAttempted,
				contracts.PrepTargetStatusNotAttempted,
			),
			exactHit:   true,
			expectSkip: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rr := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/nodes/node_xxx/claim", nil)
			resolver := &stubGateProfileResolver{
				resolution: &GateProfileResolution{
					ProfileID: 88,
					Payload:   tc.profileJSON,
					ExactHit:  tc.exactHit,
				},
			}

			if err := buildAndSendJobClaimResponse(
				rr,
				req,
				&jobStore{},
				&ConfigHolder{},
				baseRun,
				tc.spec,
				baseRunRepo,
				"https://github.com/acme/repo.git",
				baseJob,
				resolver,
			); err != nil {
				t.Fatalf("buildAndSendJobClaimResponse() error = %v", err)
			}

			var resp map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			rawSkip, ok := resp["gate_skip"]
			if !tc.expectSkip {
				if ok && rawSkip != nil {
					t.Fatalf("gate_skip=%v, want nil", rawSkip)
				}
				return
			}
			if !ok || rawSkip == nil {
				t.Fatal("expected gate_skip object, got nil")
			}
			skip, ok := rawSkip.(map[string]any)
			if !ok {
				t.Fatalf("expected gate_skip object, got %T", rawSkip)
			}
			if got := skip["enabled"]; got != true {
				t.Fatalf("gate_skip.enabled=%v, want true", got)
			}
			if got := int64(skip["source_profile_id"].(float64)); got != 88 {
				t.Fatalf("gate_skip.source_profile_id=%d, want 88", got)
			}
			if got := skip["matched_target"]; got != tc.expectMatched {
				t.Fatalf("gate_skip.matched_target=%v, want %q", got, tc.expectMatched)
			}
		})
	}
}

func gateProfilePayloadForTest(active, buildStatus, unitStatus, allTestsStatus string) []byte {
	return []byte(`{
		"schema_version": 1,
		"repo_id": "repo_1",
		"runner_mode": "simple",
		"stack": {"language": "java", "tool": "gradle", "release": "11"},
		"targets": {
			"active": "` + active + `",
			"build": {"status": "` + buildStatus + `", "command": "echo build", "env": {}},
			"unit": {"status": "` + unitStatus + `", "command": "echo unit", "env": {}},
			"all_tests": {"status": "` + allTestsStatus + `", "command": "echo all", "env": {}}
		},
		"orchestration": {"pre": [], "post": []}
	}`)
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/recovery"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const healingTestRepoSHAIn = "0123456789abcdef0123456789abcdef01234567"

func TestMaybeCreateHealingJobs_FirstAttemptCreatesJobs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	hc := newHealingChain(t,
		withHealingMeta([]byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","confidence":0.8,"reason":"docker socket missing"}}}`)),
	)

	if err := maybeCreateHealingJobs(ctx, hc.Store, nil, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if len(hc.Store.createJob.calls) != 3 {
		t.Fatalf("expected 3 CreateJob calls (heal + retry sbom + re-gate), got %d", len(hc.Store.createJob.calls))
	}

	byName := createJobsByName(hc.Store.createJob.calls)

	healJob := byName["heal-1-0"]
	if healJob.Status != domaintypes.JobStatusQueued {
		t.Fatalf("expected heal-1-0 status=Queued, got %s", healJob.Status)
	}
	if healJob.JobImage != "amata:latest" {
		t.Fatalf("expected heal-1-0 image=amata:latest, got %q", healJob.JobImage)
	}
	if healJob.NextID == nil {
		t.Fatalf("expected heal-1-0 next_id to be set")
	}
	if healJob.RepoShaIn != healingTestRepoSHAIn {
		t.Fatalf("expected heal-1-0 repo_sha_in=%q, got %q", healingTestRepoSHAIn, healJob.RepoShaIn)
	}

	reGateJob := byName["re-gate-1"]
	var retrySBOM *store.CreateJobParams
	for i := range hc.Store.createJob.calls {
		if hc.Store.createJob.calls[i].JobType == domaintypes.JobTypeSBOM {
			retrySBOM = &hc.Store.createJob.calls[i]
			break
		}
	}
	if reGateJob.Status != domaintypes.JobStatusCreated {
		t.Fatalf("expected re-gate-1 status=Created, got %s", reGateJob.Status)
	}
	if retrySBOM == nil {
		t.Fatal("expected retry sbom job to be created")
	}
	if healJob.NextID == nil || *healJob.NextID != retrySBOM.ID {
		t.Fatalf("expected heal to point to retry sbom")
	}
	if retrySBOM.NextID == nil || *retrySBOM.NextID != reGateJob.ID {
		t.Fatalf("expected retry sbom to point to re-gate")
	}
	if reGateJob.NextID == nil || *reGateJob.NextID != hc.SuccessorID {
		t.Fatalf("expected re-gate to preserve original successor %s", hc.SuccessorID)
	}
	retrySBOMMeta, err := contracts.UnmarshalJobMeta(retrySBOM.Meta)
	if err != nil {
		t.Fatalf("unmarshal retry sbom meta: %v", err)
	}
	if retrySBOMMeta.SBOM == nil || strings.TrimSpace(retrySBOMMeta.SBOM.CycleName) != "re-gate-1" {
		t.Fatalf("expected retry sbom cycle_name=re-gate-1, got %#v", retrySBOMMeta.SBOM)
	}
	if len(hc.Store.updateJobNextIDParams) != 1 {
		t.Fatalf("expected one next_id rewiring update, got %d", len(hc.Store.updateJobNextIDParams))
	}
	if hc.Store.updateJobNextIDParams[0].ID != hc.FailedJob.ID {
		t.Fatalf("expected failed job %s to be rewired, got %s", hc.FailedJob.ID, hc.Store.updateJobNextIDParams[0].ID)
	}
	if hc.Store.updateJobNextIDParams[0].NextID == nil || *hc.Store.updateJobNextIDParams[0].NextID != healJob.ID {
		t.Fatalf("expected failed job to point to heal job %s", healJob.ID)
	}

	reGateMeta, err := contracts.UnmarshalJobMeta(reGateJob.Meta)
	if err != nil {
		t.Fatalf("unmarshal re-gate meta: %v", err)
	}
	if reGateMeta.RecoveryMetadata == nil || reGateMeta.RecoveryMetadata.ErrorKind != "infra" {
		t.Fatalf("expected re-gate recovery.error_kind=infra, got %#v", reGateMeta.RecoveryMetadata)
	}
	if got, want := reGateMeta.RecoveryMetadata.StrategyID, "infra-default"; got != want {
		t.Fatalf("re-gate recovery.strategy_id = %q, want %q", got, want)
	}

	healMeta, err := contracts.UnmarshalJobMeta(healJob.Meta)
	if err != nil {
		t.Fatalf("unmarshal heal meta: %v", err)
	}
	if healMeta.RecoveryMetadata == nil || healMeta.RecoveryMetadata.ErrorKind != "infra" {
		t.Fatalf("expected heal recovery.error_kind=infra, got %#v", healMeta.RecoveryMetadata)
	}
}

func TestMaybeCreateHealingJobs_SecondAttemptUsesExistingHealJobs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	gateMeta := []byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`)
	hc := newHealingChain(t,
		withHealingMeta(gateMeta),
		withHealingSpec(func(t *testing.T) []byte { return buildHealingSpec(t, 3) }),
		withPriorHeals(
			priorHealJob{Name: "heal-1-0", JobType: domaintypes.JobTypeHeal, Status: domaintypes.JobStatusSuccess, Meta: []byte(`{}`)},
			priorHealJob{Name: "re-gate-1", JobType: domaintypes.JobTypeReGate, Status: domaintypes.JobStatusFail, ShaIn: healingTestRepoSHAIn, Meta: gateMeta},
		),
	)

	if err := maybeCreateHealingJobs(ctx, hc.Store, nil, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if len(hc.Store.createJob.calls) != 3 {
		t.Fatalf("expected 3 CreateJob calls (heal + retry sbom + re-gate), got %d", len(hc.Store.createJob.calls))
	}
	if hc.Store.createJob.calls[0].Name != "re-gate-2" {
		t.Fatalf("expected first healing job name re-gate-2, got %q", hc.Store.createJob.calls[0].Name)
	}
	if hc.Store.createJob.calls[0].JobType != domaintypes.JobTypeReGate {
		t.Fatalf("expected first job JobType=re_gate, got %q", hc.Store.createJob.calls[0].JobType)
	}
	if hc.Store.createJob.calls[1].JobType != domaintypes.JobTypeSBOM {
		t.Fatalf("expected second job JobType=sbom, got %q", hc.Store.createJob.calls[1].JobType)
	}
	retrySBOMMeta, err := contracts.UnmarshalJobMeta(hc.Store.createJob.calls[1].Meta)
	if err != nil {
		t.Fatalf("unmarshal retry sbom meta: %v", err)
	}
	if retrySBOMMeta.SBOM == nil || strings.TrimSpace(retrySBOMMeta.SBOM.CycleName) != "re-gate-2" {
		t.Fatalf("expected retry sbom cycle_name=re-gate-2, got %#v", retrySBOMMeta.SBOM)
	}
	if hc.Store.createJob.calls[2].Name != "heal-2-0" {
		t.Fatalf("expected third healing job name heal-2-0, got %q", hc.Store.createJob.calls[2].Name)
	}
	if hc.Store.createJob.calls[2].JobType != domaintypes.JobTypeHeal {
		t.Fatalf("expected third job JobType=heal, got %q", hc.Store.createJob.calls[2].JobType)
	}
	if hc.Store.createJob.calls[0].NextID == nil || *hc.Store.createJob.calls[0].NextID != hc.SuccessorID {
		t.Fatalf("expected re-gate-2 to link back to original successor %s", hc.SuccessorID)
	}
	if hc.Store.createJob.calls[1].NextID == nil || *hc.Store.createJob.calls[1].NextID != hc.Store.createJob.calls[0].ID {
		t.Fatalf("expected retry sbom to link to re-gate-2")
	}
	if hc.Store.createJob.calls[2].NextID == nil || *hc.Store.createJob.calls[2].NextID != hc.Store.createJob.calls[1].ID {
		t.Fatalf("expected heal-2-0 to link to retry sbom")
	}
}

func TestMaybeCreateHealingJobs_ReGateHooksScheduledOncePerCycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const (
		hookHashDirect = "a1b2c3d4e5f6"
		hookHashA      = "b1c2d3e4f5a6"
		hookHashB      = "c1d2e3f4a5b6"
	)

	hc := newHealingChain(t,
		withHealingSpec(func(t *testing.T) []byte {
			t.Helper()
			spec := map[string]any{
				"hooks": []any{hookHashDirect, hookHashA, hookHashB},
				"bundle_map": map[string]any{
					hookHashDirect: "bundle_heal_hooks",
					hookHashA:      "bundle_heal_hooks",
					hookHashB:      "bundle_heal_hooks",
				},
				"steps": []any{map[string]any{"image": "migs-orw:latest"}},
				"build_gate": map[string]any{
					"heal": map[string]any{
						"retries": float64(2),
						"image":   "amata:latest",
					},
				},
			}
			raw, err := json.Marshal(spec)
			if err != nil {
				t.Fatalf("marshal healing spec with hooks: %v", err)
			}
			return raw
		}),
	)
	bs := bsmock.New()
	seedPlanningHookBundle(t, hc.Store, bs, "bundle_heal_hooks", `
id: hook-bundle
steps:
  - image: hook:latest
`)
	bp := blobpersist.New(hc.Store, bs)

	if err := maybeCreateHealingJobs(ctx, hc.Store, bp, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if len(hc.Store.createJob.calls) != 3 {
		t.Fatalf("expected 3 CreateJob calls (heal + sbom + re-gate), got %d", len(hc.Store.createJob.calls))
	}

	byName := createJobsByName(hc.Store.createJob.calls)
	healJob := byName["heal-1-0"]
	reGate := byName["re-gate-1"]

	var retrySBOM *store.CreateJobParams
	for i := range hc.Store.createJob.calls {
		if hc.Store.createJob.calls[i].JobType == domaintypes.JobTypeSBOM {
			retrySBOM = &hc.Store.createJob.calls[i]
			break
		}
	}
	if retrySBOM == nil {
		t.Fatal("expected retry sbom job")
	}
	if healJob.NextID == nil || *healJob.NextID != retrySBOM.ID {
		t.Fatalf("expected heal to point to retry sbom")
	}
	if retrySBOM.NextID == nil || *retrySBOM.NextID != reGate.ID {
		t.Fatalf("expected retry sbom to point to re-gate")
	}
	if reGate.NextID == nil || *reGate.NextID != hc.SuccessorID {
		t.Fatalf("expected re-gate to preserve successor %s", hc.SuccessorID)
	}

	for _, created := range hc.Store.createJob.calls {
		if strings.Contains(created.Name, "-hook-") {
			t.Fatalf("did not expect preplanned re-gate hook job %q", created.Name)
		}
	}
}

func TestMaybeCreateHealingJobs_ReGateHooksConditionalPlanning_Mixed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.HandleFunc("/hook-go.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
id: hook-go
stack:
  language: go
steps:
  - image: hook:latest
`))
	})
	mux.HandleFunc("/hook-java.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
id: hook-java
stack:
  language: java
steps:
  - image: hook:latest
`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	hc := newHealingChain(t,
		withHealingSpec(func(t *testing.T) []byte {
			t.Helper()
			spec := map[string]any{
				"hooks": []any{
					server.URL + "/hook-go.yaml",
					server.URL + "/hook-java.yaml",
				},
				"steps": []any{map[string]any{"image": "migs-orw:latest"}},
				"build_gate": map[string]any{
					"post": map[string]any{
						"stack": map[string]any{
							"enabled":  true,
							"language": "go",
							"release":  "1.22",
						},
					},
					"heal": map[string]any{
						"retries": float64(2),
						"image":   "amata:latest",
					},
				},
			}
			raw, err := json.Marshal(spec)
			if err != nil {
				t.Fatalf("marshal healing spec with conditional hooks: %v", err)
			}
			return raw
		}),
	)

	if err := maybeCreateHealingJobs(ctx, hc.Store, nil, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if len(hc.Store.createJob.calls) != 3 {
		t.Fatalf("expected 3 CreateJob calls (heal + sbom + re-gate), got %d", len(hc.Store.createJob.calls))
	}

	byName := createJobsByName(hc.Store.createJob.calls)
	healJob := byName["heal-1-0"]
	reGate := byName["re-gate-1"]

	if _, exists := byName["re-gate-1-hook-000"]; exists {
		t.Fatal("did not expect re-gate hook jobs to be preplanned")
	}
	var retrySBOM *store.CreateJobParams
	for i := range hc.Store.createJob.calls {
		if hc.Store.createJob.calls[i].JobType == domaintypes.JobTypeSBOM {
			retrySBOM = &hc.Store.createJob.calls[i]
			break
		}
	}
	if retrySBOM == nil {
		t.Fatal("expected retry sbom job")
	}
	if healJob.NextID == nil || *healJob.NextID != retrySBOM.ID {
		t.Fatalf("expected heal to point to retry sbom")
	}
	if retrySBOM.NextID == nil || *retrySBOM.NextID != reGate.ID {
		t.Fatalf("expected retry sbom to point to re-gate")
	}
}

func TestMaybeCreateHealingJobs_ReGateHooksConditionalPlanning_AllFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.HandleFunc("/hook-java.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
id: hook-java
stack:
  language: java
steps:
  - image: hook:latest
`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	hc := newHealingChain(t,
		withHealingSpec(func(t *testing.T) []byte {
			t.Helper()
			spec := map[string]any{
				"hooks": []any{
					server.URL + "/hook-java.yaml",
				},
				"steps": []any{map[string]any{"image": "migs-orw:latest"}},
				"build_gate": map[string]any{
					"post": map[string]any{
						"stack": map[string]any{
							"enabled":  true,
							"language": "go",
							"release":  "1.22",
						},
					},
					"heal": map[string]any{
						"retries": float64(2),
						"image":   "amata:latest",
					},
				},
			}
			raw, err := json.Marshal(spec)
			if err != nil {
				t.Fatalf("marshal healing spec with all-false hooks: %v", err)
			}
			return raw
		}),
	)

	if err := maybeCreateHealingJobs(ctx, hc.Store, nil, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if len(hc.Store.createJob.calls) != 3 {
		t.Fatalf("expected 3 CreateJob calls (heal + sbom + re-gate), got %d", len(hc.Store.createJob.calls))
	}
	for _, created := range hc.Store.createJob.calls {
		if strings.Contains(created.Name, "-hook-") {
			t.Fatalf("did not expect hook job %q when all re-gate matcher decisions are false", created.Name)
		}
	}
}

func TestMaybeCreateHealingJobs_ReGateHashHooksConditionalPlanning_AllFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const hookHash = "deafbeefcafe"

	hc := newHealingChain(t,
		withHealingSpec(func(t *testing.T) []byte {
			t.Helper()
			spec := map[string]any{
				"hooks": []any{hookHash},
				"bundle_map": map[string]any{
					hookHash: "bundle_heal_hash_false",
				},
				"steps": []any{map[string]any{"image": "migs-orw:latest"}},
				"build_gate": map[string]any{
					"post": map[string]any{
						"stack": map[string]any{
							"enabled":  true,
							"language": "go",
							"release":  "1.22",
						},
					},
					"heal": map[string]any{
						"retries": float64(2),
						"image":   "amata:latest",
					},
				},
			}
			raw, err := json.Marshal(spec)
			if err != nil {
				t.Fatalf("marshal healing spec with hash all-false hooks: %v", err)
			}
			return raw
		}),
	)
	bs := bsmock.New()
	seedPlanningHookBundle(t, hc.Store, bs, "bundle_heal_hash_false", `
id: hook-ruby
stack:
  language: ruby
steps:
  - image: hook:latest
`)
	bp := blobpersist.New(hc.Store, bs)

	if err := maybeCreateHealingJobs(ctx, hc.Store, bp, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if len(hc.Store.createJob.calls) != 3 {
		t.Fatalf("expected 3 CreateJob calls (heal + sbom + re-gate), got %d", len(hc.Store.createJob.calls))
	}
	for _, created := range hc.Store.createJob.calls {
		if strings.Contains(created.Name, "-hook-") {
			t.Fatalf("did not expect hook job %q when all re-gate matcher decisions are false", created.Name)
		}
	}
}

// TestMaybeCreateHealingJobs_CancelsRemaining covers cases where healing cannot proceed
// and the successor must be cancelled instead.
func TestMaybeCreateHealingJobs_CancelsRemaining(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		errorKind   string
		repoShaIn   string
		specSetup   func(st *jobStore)
		expectErr   string
		extraAssert func(t *testing.T, st *jobStore)
	}{
		{
			name:      "no_heal_config",
			errorKind: "infra",
			repoShaIn: healingTestRepoSHAIn,
			specSetup: func(st *jobStore) {
				st.getSpec.val = store.Spec{
					ID:   st.getSpec.val.ID,
					Spec: []byte(`{"steps":[{"image":"migs-orw:latest"}],"build_gate":{"enabled":true}}`),
				}
			},
		},
		{
			name:      "invalid_repo_sha_in",
			errorKind: "infra",
			repoShaIn: "invalid",
		},
		{
			name:      "spec_fetch_error",
			errorKind: "infra",
			repoShaIn: healingTestRepoSHAIn,
			specSetup: func(st *jobStore) {
				st.getSpec.err = errors.New("db unavailable")
			},
			expectErr: "get spec: db unavailable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			hc := newHealingChain(t,
				withHealingMeta([]byte(`{"kind":"gate","gate":{"recovery":{"loop_kind":"healing","error_kind":"`+tc.errorKind+`","strategy_id":"`+tc.errorKind+`-default"}}}`)),
				withHealingRepoShaIn(tc.repoShaIn),
			)
			if tc.specSetup != nil {
				tc.specSetup(hc.Store)
			}

			err := maybeCreateHealingJobs(ctx, hc.Store, nil, hc.Run, hc.FailedJob)
			if tc.expectErr != "" {
				if err == nil || err.Error() != tc.expectErr {
					t.Fatalf("maybeCreateHealingJobs error = %v, want %q", err, tc.expectErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
			}

			assertCancelsSuccessor(t, hc.Store, hc.SuccessorID)

			if tc.extraAssert != nil {
				tc.extraAssert(t, hc.Store)
			}
		})
	}
}

func TestMaybeCreateHealingJobs_ReGateInfraCandidateValidatedFromPreviousHeal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	reGateMeta := []byte(`{"kind":"gate","gate":{"static_checks":[{"language":"java","tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}}}`)
	hc := newHealingChain(t,
		withHealingMeta(nil), // pre-gate has no meta in this scenario
		withHealingSpec(func(t *testing.T) []byte { return buildHealingSpec(t, 3, withArtifactExpectations()) }),
		withPriorHeals(
			priorHealJob{Name: "heal-1-0", JobType: domaintypes.JobTypeHeal, Status: domaintypes.JobStatusSuccess, Meta: []byte(`{"kind":"mig"}`)},
			priorHealJob{Name: "re-gate-1", JobType: domaintypes.JobTypeReGate, Status: domaintypes.JobStatusFail, ShaIn: healingTestRepoSHAIn, Meta: reGateMeta},
		),
	)

	// Set up blob store with candidate artifact from the prior heal job.
	heal1ID := hc.Jobs[1].ID // heal-1-0
	objKey := "artifacts/run/" + hc.RunID.String() + "/bundle/heal-1.tar.gz"
	hc.Store.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
		{RunID: hc.RunID, JobID: &heal1ID, ObjectKey: ptr(objKey)},
	}

	candidateJSON := []byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"java","tool":"maven"},
		"targets": {
			"active": "build",
			"build": {"status":"passed","command":"go test ./...","env":{},"failure_code":null},
			"unit": {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []},
		"tactics_used": ["go_default"],
		"attempts": [],
		"evidence": {"log_refs": ["inline://prep/test"], "diagnostics": []},
		"repro_check": {"status":"passed","details":"ok"},
		"prompt_delta_suggestion": {"status":"none","summary":"","candidate_lines":[]}
	}`)
	bs := bsmock.New()
	bundle := mustTarGzPayload(t, map[string][]byte{
		"out/gate-profile-candidate.json": candidateJSON,
	})
	if _, err := bs.Put(ctx, objKey, "application/gzip", bundle); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	bp := blobpersist.New(hc.Store, bs)

	if err := maybeCreateHealingJobs(ctx, hc.Store, bp, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}
	if len(hc.Store.createJob.calls) != 3 {
		t.Fatalf("expected 3 CreateJob calls (re-gate + retry sbom + heal), got %d", len(hc.Store.createJob.calls))
	}

	createdReGateMeta, err := contracts.UnmarshalJobMeta(hc.Store.createJob.calls[0].Meta)
	if err != nil {
		t.Fatalf("unmarshal re-gate meta: %v", err)
	}
	if createdReGateMeta.RecoveryMetadata == nil {
		t.Fatal("expected recovery metadata")
	}
	if got, want := createdReGateMeta.RecoveryMetadata.CandidateSchemaID, contracts.GateProfileCandidateSchemaID; got != want {
		t.Fatalf("candidate_schema_id = %q, want %q", got, want)
	}
	if got, want := createdReGateMeta.RecoveryMetadata.CandidateArtifactPath, contracts.GateProfileCandidateArtifactPath; got != want {
		t.Fatalf("candidate_artifact_path = %q, want %q", got, want)
	}
	if got, want := createdReGateMeta.RecoveryMetadata.CandidateValidationStatus, contracts.RecoveryCandidateStatusValid; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q (error=%q)", got, want, createdReGateMeta.RecoveryMetadata.CandidateValidationError)
	}
	if len(createdReGateMeta.RecoveryMetadata.CandidateGateProfile) == 0 {
		t.Fatal("expected candidate_gate_profile to be stored")
	}
}

func TestMaybeCreateHealingJobs_FirstInsertionInfraCandidateMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	hc := newHealingChain(t,
		withHealingSpec(func(t *testing.T) []byte { return buildHealingSpec(t, 2, withArtifactExpectations()) }),
	)

	if err := maybeCreateHealingJobs(ctx, hc.Store, nil, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	var reGateMetaRaw []byte
	for i := range hc.Store.createJob.calls {
		if hc.Store.createJob.calls[i].JobType == domaintypes.JobTypeReGate {
			reGateMetaRaw = hc.Store.createJob.calls[i].Meta
			break
		}
	}
	if len(reGateMetaRaw) == 0 {
		t.Fatal("expected re-gate job metadata")
	}
	createdReGateMeta, err := contracts.UnmarshalJobMeta(reGateMetaRaw)
	if err != nil {
		t.Fatalf("unmarshal re-gate meta: %v", err)
	}
	if createdReGateMeta.RecoveryMetadata == nil {
		t.Fatal("expected recovery metadata")
	}
	if got, want := createdReGateMeta.RecoveryMetadata.CandidateValidationStatus, contracts.RecoveryCandidateStatusMissing; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q", got, want)
	}
	if createdReGateMeta.RecoveryMetadata.CandidateValidationError == "" {
		t.Fatal("expected candidate_validation_error for missing candidate")
	}
}

func TestMaybeCompleteMultiStepRun_FinishesWhenAllReposTerminal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()

	st := &jobStore{}
	st.countRunReposByStatus.val = []store.CountRunReposByStatusRow{
		{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
		{Status: domaintypes.RunRepoStatusFail, Count: 1},
	}

	run := store.Run{ID: runID, Status: domaintypes.RunStatusStarted}
	if _, err := recovery.MaybeCompleteRunIfAllReposTerminal(ctx, st, nil, run); err != nil {
		t.Fatalf("maybeCompleteRunIfAllReposTerminal returned error: %v", err)
	}

	if !st.updateRunStatus.called {
		t.Fatalf("expected UpdateRunStatus to be called")
	}
	if st.updateRunStatus.params.ID != runID || st.updateRunStatus.params.Status != domaintypes.RunStatusFinished {
		t.Fatalf("unexpected UpdateRunStatus params: %+v", st.updateRunStatus.params)
	}
}

func TestLoadRecoveryArtifact_Success(t *testing.T) {
	t.Parallel()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	objKey := "artifacts/run/" + runID.String() + "/bundle/test.tar.gz"

	st := &jobStore{}
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
		{
			RunID:     runID,
			JobID:     &jobID,
			ObjectKey: ptr(objKey),
		},
	}
	bs := bsmock.New()
	bundle := mustTarGzPayload(t, map[string][]byte{
		"out/gate-profile-candidate.json": []byte(`{"schema_version":1}`),
	})
	if _, err := bs.Put(context.Background(), objKey, "application/gzip", bundle); err != nil {
		t.Fatalf("put blob: %v", err)
	}

	bp := blobpersist.New(st, bs)
	raw, err := loadRecoveryArtifact(context.Background(), st, bp, runID, jobID, "/out/gate-profile-candidate.json")
	if err != nil {
		t.Fatalf("loadRecoveryArtifact error: %v", err)
	}
	if string(raw) != `{"schema_version":1}` {
		t.Fatalf("unexpected payload: %s", string(raw))
	}
}

func TestLoadRecoveryArtifact_TypedErrors(t *testing.T) {
	t.Parallel()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	st := &jobStore{}
	bp := blobpersist.New(st, bsmock.New())

	_, err := loadRecoveryArtifact(context.Background(), st, bp, runID, jobID, "/out/gate-profile-candidate.json")
	if !errors.Is(err, blobpersist.ErrRecoveryArtifactNotFound) {
		t.Fatalf("expected ErrRecoveryArtifactNotFound, got %v", err)
	}
}

func TestCandidateMatchesDetectedStack_ReleaseAware(t *testing.T) {
	t.Parallel()

	profile, err := contracts.ParseGateProfileJSON([]byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"java","tool":"gradle","release":"11"},
		"targets": {
			"active": "build",
			"build": {"status":"passed","command":"./gradlew test","env":{},"failure_code":null},
			"unit": {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []},
		"tactics_used": [],
		"attempts": [],
		"evidence": {"log_refs": [], "diagnostics": []},
		"repro_check": {"status": "failed", "details": ""},
		"prompt_delta_suggestion": {"status":"none","summary":"","candidate_lines":[]}
	}`))
	if err != nil {
		t.Fatalf("ParseGateProfileJSON: %v", err)
	}

	if !candidateMatchesDetectedStack(profile, &contracts.StackExpectation{
		Language: "java",
		Tool:     "gradle",
		Release:  "11",
	}) {
		t.Fatal("expected exact release match to pass")
	}
	if candidateMatchesDetectedStack(profile, &contracts.StackExpectation{
		Language: "java",
		Tool:     "gradle",
		Release:  "17",
	}) {
		t.Fatal("expected mismatched release to fail")
	}
	if !candidateMatchesDetectedStack(profile, &contracts.StackExpectation{
		Language: "java",
		Tool:     "gradle",
		Release:  "",
	}) {
		t.Fatal("expected empty detected release to act as wildcard")
	}
}

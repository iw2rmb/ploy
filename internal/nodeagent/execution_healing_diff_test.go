package nodeagent

import (
	"context"
	"os"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// TestExecuteWithHealing_RepoDiffSemantics verifies that healing verification aligns
// with the HTTP Build Gate API's repo+diff model:
//   - Pre-gate validates the workspace (repo_url+ref clone)
//   - Healing mods modify the workspace in-place
//   - Re-gate validates workspace = repo_url+ref + healing modifications
//
// This ensures conceptual equivalence with POST /v1/buildgate/validate using diff_patch.
func TestExecuteWithHealing_RepoDiffSemantics(t *testing.T) {
	// Track workspace paths passed to gate executor to verify same workspace is reused.
	var gateWorkspaces []string
	gateCallCount := 0

	// Mock gate executor that captures workspace paths and tracks call sequence.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			gateWorkspaces = append(gateWorkspaces, workspace)

			if gateCallCount == 1 {
				// Pre-gate fails (initial repo_url+ref validation).
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] Missing symbol: UnknownClass\n",
				}, nil
			}
			// Re-gate passes after healing modifications.
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText: "[INFO] BUILD SUCCESS\n",
			}, nil
		},
	}

	// Track container specs to verify healing mod receives workspace.
	var containerSpecs []step.ContainerSpec
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			containerSpecs = append(containerSpecs, spec)
			return step.ContainerHandle{ID: "mock-container"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("healing complete"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

	// Create workspace simulating repo_url+ref clone.
	workspace, err := os.MkdirTemp("", "ploy-test-repo-diff-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	outDir, err := os.MkdirTemp("", "ploy-test-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

	// Request with repo metadata (matching the repo+diff model).
	req := StartRunRequest{
		RunID:     types.RunID("test-run-repo-diff"),
		JobID:     types.JobID("test-job-repo-diff"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("e2e/fail-missing-symbol"),
		TargetRef: types.GitRef("mods-upgrade-java17"),
		CommitSHA: types.CommitSHA("abc123"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     ModContainerSpec{Image: contracts.ModImage{Universal: "test/codex-healer:latest"}},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Main mod",
		Image: "test/main-mod:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "workspace",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadWrite,
				SnapshotCID: types.CID("bafy123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
		Options: map[string]any{},
	}

	result, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)

	// Should succeed after healing.
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}

	// Verify repo+diff semantics: all gates (pre-gate, pre-mod re-gate, post-mod gate) use the SAME workspace path.
	// With post-mod gate, we now have 3 gate calls: pre-gate, pre-mod re-gate, post-mod gate.
	if len(gateWorkspaces) != 3 {
		t.Fatalf("expected 3 gate calls (pre-gate, pre-mod re-gate, post-mod gate), got %d", len(gateWorkspaces))
	}

	// All gate calls should use the same workspace.
	for i := 1; i < len(gateWorkspaces); i++ {
		if gateWorkspaces[0] != gateWorkspaces[i] {
			t.Errorf("repo+diff semantics violation: gate workspace[0] %q != gate workspace[%d] %q; "+
				"all gates must reuse the same workspace containing repo_url+ref + changes",
				gateWorkspaces[0], i, gateWorkspaces[i])
		}
	}

	// Both should point to our test workspace (the repo_url+ref clone).
	if gateWorkspaces[0] != workspace {
		t.Errorf("gate workspace = %q, want %q (original repo_url+ref workspace)", gateWorkspaces[0], workspace)
	}

	// Verify healing container also received the same workspace to modify.
	// This ensures healing mods accumulate changes on top of the repo baseline.
	if len(containerSpecs) < 1 {
		t.Fatal("expected at least one container spec for healing mod")
	}

	healerWorkspace := ""
	for _, mount := range containerSpecs[0].Mounts {
		if mount.Target == "/workspace" {
			healerWorkspace = mount.Source
			break
		}
	}
	if healerWorkspace != workspace {
		t.Errorf("healing mod workspace = %q, want %q (same repo_url+ref workspace)", healerWorkspace, workspace)
	}
}

// trackingDiffGenerator is a test helper that records Generate/GenerateBetween calls.
type trackingDiffGenerator struct {
	diffContent     []byte
	generateCalled  bool
	generateBetween bool
	lastBaseDir     string
	lastModifiedDir string
}

func (t *trackingDiffGenerator) Generate(ctx context.Context, workspace string) ([]byte, error) {
	t.generateCalled = true
	return t.diffContent, nil
}

func (t *trackingDiffGenerator) GenerateBetween(ctx context.Context, baseDir, modifiedDir string) ([]byte, error) {
	t.generateBetween = true
	t.lastBaseDir = baseDir
	t.lastModifiedDir = modifiedDir
	return t.diffContent, nil
}

// TestUploadHealingJobDiff_UsesGenerateBetween verifies that discrete healing jobs
// use GenerateBetween (repo+diff semantics) and still publish mod_type="mod" diffs.
func TestUploadHealingJobDiff_UsesGenerateBetween(t *testing.T) {
	t.Parallel()

	workspace, err := os.MkdirTemp("", "ploy-heal-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	baseDir, err := os.MkdirTemp("", "ploy-heal-base-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(baseDir) }()

	diffGen := &trackingDiffGenerator{
		diffContent: []byte("healing-diff"),
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

	result := step.Result{
		ExitCode: 0,
		Timings:  step.StageTiming{},
	}

	rc.uploadHealingJobDiff(context.Background(), "run-1", "job-1", "heal-1-0", diffGen, baseDir, workspace, result, 1500)

	if !diffGen.generateBetween {
		t.Fatalf("GenerateBetween was not called for healing job diff")
	}
	if diffGen.generateCalled {
		t.Fatalf("Generate should not be called when baseline is provided")
	}
	if diffGen.lastBaseDir != baseDir {
		t.Errorf("GenerateBetween baseDir = %q, want %q", diffGen.lastBaseDir, baseDir)
	}
	if diffGen.lastModifiedDir != workspace {
		t.Errorf("GenerateBetween modifiedDir = %q, want %q", diffGen.lastModifiedDir, workspace)
	}
}

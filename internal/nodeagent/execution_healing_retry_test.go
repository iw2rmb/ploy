package nodeagent

import (
	"context"
	"errors"
	"os"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// Healing retry and edge-path tests focused on the gate-heal-regate loop.

// TestExecuteWithHealing_ModNonZeroExit_DoesNotAbort ensures a healing mod returning
// a non-zero exit code does not abort the loop; the gate is still re-run.
func TestExecuteWithHealing_ModNonZeroExit_DoesNotAbort(t *testing.T) {
	gateCallCount := 0
	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		gateCallCount++
		// First gate fails, second gate passes after healing (even though healer exits non-zero).
		passed := gateCallCount > 1
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "java", Passed: passed}}, LogsText: "gate"}, nil
	}}

	// Healing container exits with non-zero; main mod exits with zero.
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			id := "main"
			if spec.Image == "heal:latest" {
				id = "heal"
			}
			return step.ContainerHandle{ID: id}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			if handle.ID == "heal" {
				return step.ContainerResult{ExitCode: 17}, nil
			}
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return []byte(""), nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	ws, _ := os.MkdirTemp("", "ploy-ws-*")
	defer os.RemoveAll(ws)
	outDir, _ := os.MkdirTemp("", "ploy-out-*")
	defer os.RemoveAll(outDir)
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "http://localhost", NodeID: "n"}}

	req := StartRunRequest{RunID: types.RunID("t-nonzero"), RepoURL: types.RepoURL("https://gitlab.com/acme/x.git"), BaseRef: types.GitRef("main"), TargetRef: types.GitRef("br"), Options: map[string]any{
		"build_gate_healing": map[string]any{"retries": 1, "mods": []any{map[string]any{"image": "heal:latest"}}},
	}}
	manifest := contracts.StepManifest{ID: types.StepID(req.RunID), Image: "main:latest", Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}}, Gate: &contracts.StepGateSpec{Enabled: true, Profile: "java"}, Options: req.Options}

	res, err := rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir)
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("main mod not executed successfully: exit=%d", res.ExitCode)
	}
	if gateCallCount != 2 {
		t.Fatalf("expected 2 gate calls (pre + re), got %d", gateCallCount)
	}
}

// TestExecuteWithHealing_RetriesFloat64ValueHonored verifies that a JSON-typed
// float64 for retries is respected (e.g., when unmarshalled from generic JSON).
func TestExecuteWithHealing_RetriesFloat64ValueHonored(t *testing.T) {
	// Gate always fails to force all retry attempts.
	gateCalls := 0
	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		gateCalls++
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "java", Passed: false}}, LogsText: "fail"}, nil
	}}

	// Count healing container creations.
	creates := 0
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			creates++
			return step.ContainerHandle{ID: "heal"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return []byte(""), nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	ws, _ := os.MkdirTemp("", "ploy-ws-*")
	defer os.RemoveAll(ws)
	outDir, _ := os.MkdirTemp("", "ploy-out-*")
	defer os.RemoveAll(outDir)
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "http://localhost", NodeID: "n"}}

	// Use float64 for retries as produced by encoding/json when decoding into map[string]any.
	req := StartRunRequest{RunID: types.RunID("t-f64"), RepoURL: types.RepoURL("https://gitlab.com/acme/x.git"), BaseRef: types.GitRef("main"), TargetRef: types.GitRef("br"), Options: map[string]any{
		"build_gate_healing": map[string]any{"retries": float64(2), "mods": []any{map[string]any{"image": "heal:latest"}}},
	}}
	manifest := contracts.StepManifest{ID: types.StepID(req.RunID), Image: "main:latest", Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}}, Gate: &contracts.StepGateSpec{Enabled: true, Profile: "java"}, Options: req.Options}

	_, err := rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir)
	if err == nil || !errors.Is(err, step.ErrBuildGateFailed) {
		t.Fatalf("want ErrBuildGateFailed after retries exhausted, got %v", err)
	}

	// With retries=2 and 1 healer mod, expect 2 healing attempts and 2 re-gate calls.
	if creates != 2 {
		t.Fatalf("healing container creates=%d, want 2", creates)
	}
	if gateCalls != 3 { // 1 pre-gate + 2 re-gates
		t.Fatalf("gate calls=%d, want 3 (pre + 2 re-gates)", gateCalls)
	}
}

// TestExecuteWithHealing_HealingConfiguredNoMods_NoHealing verifies that an empty mods array
// behaves as no-healing configuration (return pre-gate failure immediately).
func TestExecuteWithHealing_HealingConfiguredNoMods_NoHealing(t *testing.T) {
	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "java", Passed: false}}, LogsText: "fail"}, nil
	}}

	mockContainer := &mockContainerRuntime{createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		t.Fatalf("no container should be created when mods are empty")
		return step.ContainerHandle{ID: "x"}, nil
	}}

	ws, _ := os.MkdirTemp("", "ploy-ws-*")
	defer os.RemoveAll(ws)
	outDir, _ := os.MkdirTemp("", "ploy-out-*")
	defer os.RemoveAll(outDir)
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "http://localhost", NodeID: "n"}}

	req := StartRunRequest{RunID: types.RunID("t-empty-mods"), RepoURL: types.RepoURL("https://gitlab.com/acme/x.git"), BaseRef: types.GitRef("main"), TargetRef: types.GitRef("br"), Options: map[string]any{
		"build_gate_healing": map[string]any{"retries": 3, "mods": []any{}},
	}}
	manifest := contracts.StepManifest{ID: types.StepID(req.RunID), Image: "main:latest", Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}}, Gate: &contracts.StepGateSpec{Enabled: true, Profile: "java"}, Options: req.Options}

	_, err := rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir)
	if err == nil || !errors.Is(err, step.ErrBuildGateFailed) {
		t.Fatalf("want ErrBuildGateFailed without healing, got %v", err)
	}
}

// TestExecuteWithHealing_InjectsServerAndTLSVars ensures TLS and server URL
// env vars are injected into healing containers for Build Gate API access.
func TestExecuteWithHealing_InjectsServerAndTLSVars(t *testing.T) {
	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		// Force healing by failing pre-gate, then pass on re-gate.
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}}, LogsText: "fail"}, nil
	}}

	var envSeen map[string]string
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			// Capture on first healing container.
			if envSeen == nil {
				envSeen = map[string]string{}
				for k, v := range spec.Env {
					envSeen[k] = v
				}
			}
			return step.ContainerHandle{ID: "heal"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return []byte(""), nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	ws, _ := os.MkdirTemp("", "ploy-ws-*")
	defer os.RemoveAll(ws)
	outDir, _ := os.MkdirTemp("", "ploy-out-*")
	defer os.RemoveAll(outDir)
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "https://server.example.com:8443", NodeID: "n", HTTP: HTTPConfig{TLS: TLSConfig{Enabled: true, CAPath: "/tmp/ca.crt", CertPath: "/tmp/cert.crt", KeyPath: "/tmp/key.key"}}}}

	req := StartRunRequest{RunID: types.RunID("t-tls"), RepoURL: types.RepoURL("https://gitlab.com/acme/x.git"), BaseRef: types.GitRef("main"), TargetRef: types.GitRef("br"), Options: map[string]any{
		"build_gate_healing": map[string]any{"retries": 1, "mods": []any{map[string]any{"image": "heal:latest"}}},
	}}
	manifest := contracts.StepManifest{ID: types.StepID(req.RunID), Image: "main:latest", Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}}, Gate: &contracts.StepGateSpec{Enabled: true, Profile: "java"}, Options: req.Options}

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir)

	if envSeen == nil {
		t.Fatal("healing env not captured")
	}
	if envSeen["PLOY_SERVER_URL"] != "https://server.example.com:8443" {
		t.Fatalf("PLOY_SERVER_URL=%q, want https://server.example.com:8443", envSeen["PLOY_SERVER_URL"])
	}
	if envSeen["PLOY_CA_CERT_PATH"] == "" || envSeen["PLOY_CLIENT_CERT_PATH"] == "" || envSeen["PLOY_CLIENT_KEY_PATH"] == "" {
		t.Fatalf("expected TLS envs to be set, got: ca=%q cert=%q key=%q", envSeen["PLOY_CA_CERT_PATH"], envSeen["PLOY_CLIENT_CERT_PATH"], envSeen["PLOY_CLIENT_KEY_PATH"])
	}
}

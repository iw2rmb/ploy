package nodeagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	defer func() { _ = os.RemoveAll(ws) }()
	outDir, _ := os.MkdirTemp("", "ploy-out-*")
	defer func() { _ = os.RemoveAll(outDir) }()
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "http://localhost", NodeID: "n"}}

	req := StartRunRequest{
		RunID:     types.RunID("t-nonzero"),
		JobID:     types.JobID("j-nonzero"),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "heal:latest"}},
			},
		},
	}
	manifest := contracts.StepManifest{ID: types.StepID(req.JobID), Image: "main:latest", Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}}, Gate: &contracts.StepGateSpec{Enabled: true}}

	res, err := rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("main mod not executed successfully: exit=%d", res.ExitCode)
	}
	// With post-mod gate enabled, we now have 3 gate calls: pre-gate, pre-mod re-gate, post-mod gate.
	if gateCallCount != 3 {
		t.Fatalf("expected 3 gate calls (pre + re + post), got %d", gateCallCount)
	}
}

// TestExecuteWithHealing_RetriesValueHonored verifies that the healing retry
// limit is enforced (retries=N → exactly N healing attempts).
func TestExecuteWithHealing_RetriesValueHonored(t *testing.T) {
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
	defer func() { _ = os.RemoveAll(ws) }()
	outDir, _ := os.MkdirTemp("", "ploy-out-*")
	defer func() { _ = os.RemoveAll(outDir) }()
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "http://localhost", NodeID: "n"}}

	req := StartRunRequest{
		RunID:     types.RunID("t-retries"),
		JobID:     types.JobID("j-retries"),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 2,
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "heal:latest"}},
			},
		},
	}
	manifest := contracts.StepManifest{ID: types.StepID(req.JobID), Image: "main:latest", Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}}, Gate: &contracts.StepGateSpec{Enabled: true}}

	_, err := rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)
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

// TestExecuteWithHealing_HealingConfiguredNoMod_NoHealing verifies that when
// build_gate_healing is present but no mod is configured, healing is treated
// as disabled (return pre-gate failure immediately).
func TestExecuteWithHealing_HealingConfiguredNoMod_NoHealing(t *testing.T) {
	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "java", Passed: false}}, LogsText: "fail"}, nil
	}}

	mockContainer := &mockContainerRuntime{createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		t.Fatalf("no container should be created when no healing mod is configured")
		return step.ContainerHandle{ID: "x"}, nil
	}}

	ws, _ := os.MkdirTemp("", "ploy-ws-*")
	defer func() { _ = os.RemoveAll(ws) }()
	outDir, _ := os.MkdirTemp("", "ploy-out-*")
	defer func() { _ = os.RemoveAll(outDir) }()
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "http://localhost", NodeID: "n"}}

	req := StartRunRequest{
		RunID:     types.RunID("t-empty-strategies"),
		JobID:     types.JobID("j-empty-strategies"),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 3,
			},
		},
	}
	manifest := contracts.StepManifest{ID: types.StepID(req.JobID), Image: "main:latest", Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}}, Gate: &contracts.StepGateSpec{Enabled: true}}

	_, err := rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)
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
	defer func() { _ = os.RemoveAll(ws) }()
	outDir, _ := os.MkdirTemp("", "ploy-out-*")
	defer func() { _ = os.RemoveAll(outDir) }()
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "http://127.0.0.1:8080", NodeID: "n", HTTP: HTTPConfig{TLS: TLSConfig{Enabled: false}}}}

	req := StartRunRequest{
		RunID:     types.RunID("t-tls"),
		JobID:     types.JobID("j-tls"),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "heal:latest"}},
			},
		},
	}
	manifest := contracts.StepManifest{ID: types.StepID(req.JobID), Image: "main:latest", Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}}, Gate: &contracts.StepGateSpec{Enabled: true}}

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	if envSeen == nil {
		t.Fatal("healing env not captured")
	}
	if envSeen["PLOY_SERVER_URL"] != "http://127.0.0.1:8080" {
		t.Fatalf("PLOY_SERVER_URL=%q, want http://127.0.0.1:8080", envSeen["PLOY_SERVER_URL"])
	}
	if envSeen["PLOY_CA_CERT_PATH"] == "" || envSeen["PLOY_CLIENT_CERT_PATH"] == "" || envSeen["PLOY_CLIENT_KEY_PATH"] == "" {
		t.Fatalf("expected TLS envs to be set, got: ca=%q cert=%q key=%q", envSeen["PLOY_CA_CERT_PATH"], envSeen["PLOY_CLIENT_CERT_PATH"], envSeen["PLOY_CLIENT_KEY_PATH"])
	}
}

// TestExecuteWithHealing_RetriesExhausted verifies that when healing retries are exhausted
// and the gate still fails, the function returns an appropriate error.
// This test validates:
//   - Retry limit enforcement (retries=2 → exactly 2 healing attempts)
//   - Error propagation when all retries fail
//   - Main mod is skipped when healing exhausts retries
func TestExecuteWithHealing_RetriesExhausted(t *testing.T) {
	// Mock gate executor that always fails to simulate persistent build issues.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: false},
				},
				LogsText: "[ERROR] Persistent build failure\n",
			}, nil
		},
	}

	// Mock container runtime with counter to ensure main mod isn't executed.
	containerCreates := 0
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			containerCreates++
			return step.ContainerHandle{ID: "mock-container"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("healer logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

	workspace, _ := os.MkdirTemp("", "ploy-test-ws-*")
	defer func() { _ = os.RemoveAll(workspace) }()

	outDir, _ := os.MkdirTemp("", "ploy-test-out-*")
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

	req := StartRunRequest{
		RunID:     types.RunID("test-run-exhausted"),
		JobID:     types.JobID("test-job-exhausted"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("test-branch"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 2, // Try twice
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "test/healer:latest"}},
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
	}

	_, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)

	// Should return build gate failure error.
	if err == nil {
		t.Fatalf("executeWithHealing() expected error, got nil")
	}

	if !errors.Is(err, step.ErrBuildGateFailed) {
		t.Errorf("executeWithHealing() error should wrap ErrBuildGateFailed, got: %v", err)
	}

	// Error should mention retries exhausted.
	if err.Error() != "build gate failed: healing retries exhausted" {
		t.Errorf("executeWithHealing() error = %q, want 'build gate failed: healing retries exhausted'", err.Error())
	}

	// With 1 healing mod and retries=2, only 2 healer containers should run; main mod should not run.
	if containerCreates != 2 {
		t.Errorf("healing containers created = %d, want 2 (main mod must be skipped)", containerCreates)
	}
}

// TestExecuteWithHealing_InjectsHostWorkspaceEnv verifies that the healing
// container receives PLOY_HOST_WORKSPACE env with the host workspace path.
// This is critical for healing mods that need to interact with the host Docker daemon.
// This test validates:
//   - PLOY_HOST_WORKSPACE environment variable injection into healing containers
//   - Docker socket mount configuration for healing containers
//   - Proper setup before retry attempts
func TestExecuteWithHealing_InjectsHostWorkspaceEnv(t *testing.T) {
	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		// Fail pre-gate to trigger healing.
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}}, LogsText: "fail"}, nil
	}}

	var capturedEnv map[string]string
	var capturedMounts []step.ContainerMount
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			// Capture env on first healing container creation.
			copied := make(map[string]string, len(spec.Env))
			for k, v := range spec.Env {
				copied[k] = v
			}
			capturedEnv = copied
			capturedMounts = append([]step.ContainerMount{}, spec.Mounts...)
			return step.ContainerHandle{ID: "heal"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return []byte(""), nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	ws, err := os.MkdirTemp("", "ploy-host-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(ws) }()

	outDir, _ := os.MkdirTemp("", "ploy-out-*")
	defer func() { _ = os.RemoveAll(outDir) }()
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "http://localhost", NodeID: "n"}}

	req := StartRunRequest{
		RunID:     types.RunID("t-env"),
		JobID:     types.JobID("j-env"),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "heal:latest"}},
			},
		},
	}

	manifest := contracts.StepManifest{ID: types.StepID(req.JobID), Image: "main:latest", Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}}, Gate: &contracts.StepGateSpec{Enabled: true}}

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	if capturedEnv == nil {
		t.Fatal("healing env not captured")
	}
	if got := capturedEnv["PLOY_HOST_WORKSPACE"]; got != ws {
		t.Fatalf("PLOY_HOST_WORKSPACE=%q, want %q", got, ws)
	}

	// Assert docker socket mount present when host socket exists.
	wantSock := false
	for _, m := range capturedMounts {
		if m.Target == "/var/run/docker.sock" && m.Source == "/var/run/docker.sock" {
			wantSock = true
			break
		}
	}
	// Do not hard-fail on platforms without the socket; check only when present.
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		if !wantSock {
			t.Fatalf("docker.sock mount not found in healing container spec")
		}
	}
}

// TestExecuteWithHealing_InjectsBearerFromEnv verifies that when PLOY_API_TOKEN
// is set in the node process environment, it is propagated into healing
// container env regardless of TLS configuration.
func TestExecuteWithHealing_InjectsBearerFromEnv(t *testing.T) {
	t.Setenv("PLOY_API_TOKEN", "env-token-123")

	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		// Fail pre-gate to trigger healing.
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}}, LogsText: "fail"}, nil
	}}

	var capturedEnv map[string]string
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			if capturedEnv == nil {
				copied := make(map[string]string, len(spec.Env))
				for k, v := range spec.Env {
					copied[k] = v
				}
				capturedEnv = copied
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

	ws, err := os.MkdirTemp("", "ploy-env-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(ws) }()

	outDir, _ := os.MkdirTemp("", "ploy-env-out-*")
	defer func() { _ = os.RemoveAll(outDir) }()
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{
		ServerURL: "http://localhost",
		NodeID:    "n",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled:  true,
				CertPath: "/etc/ploy/pki/node.crt",
				KeyPath:  "/etc/ploy/pki/node.key",
				CAPath:   "/etc/ploy/pki/ca.crt",
			},
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			IdleTimeout:  10 * time.Second,
		},
	}}

	req := StartRunRequest{
		RunID:     types.RunID("t-env-bearer"),
		JobID:     types.JobID("t-job-env-bearer"),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "heal:latest"}},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:     types.StepID(req.JobID),
		Image:  "main:latest",
		Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}},
		Gate:   &contracts.StepGateSpec{Enabled: true},
	}

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	if capturedEnv == nil {
		t.Fatal("healing env not captured")
	}
	if got := capturedEnv["PLOY_API_TOKEN"]; got != "env-token-123" {
		t.Fatalf("PLOY_API_TOKEN=%q, want env-token-123", got)
	}
}

// TestExecuteWithHealing_InjectsBearerFromFileWhenTLSEnabledFalse verifies that
// when TLS is disabled and PLOY_API_TOKEN is not set, the worker bearer token
// file is used as a fallback source for PLOY_API_TOKEN in healing containers.
func TestExecuteWithHealing_InjectsBearerFromFileWhenTLSEnabledFalse(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "ploy-node-bearer-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	if _, err := tmpFile.WriteString("file-token-abc\n"); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tmpFile.Name())
	t.Setenv("PLOY_API_TOKEN", "")

	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}}, LogsText: "fail"}, nil
	}}

	var capturedEnv map[string]string
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			if capturedEnv == nil {
				copied := make(map[string]string, len(spec.Env))
				for k, v := range spec.Env {
					copied[k] = v
				}
				capturedEnv = copied
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

	ws, err := os.MkdirTemp("", "ploy-file-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(ws) }()

	outDir, _ := os.MkdirTemp("", "ploy-file-out-*")
	defer func() { _ = os.RemoveAll(outDir) }()
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{
		ServerURL: "http://localhost",
		NodeID:    "n",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}}

	req := StartRunRequest{
		RunID:     types.RunID("t-file-bearer"),
		JobID:     types.JobID("t-job-file-bearer"),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "heal:latest"}},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:     types.StepID(req.JobID),
		Image:  "main:latest",
		Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}},
		Gate:   &contracts.StepGateSpec{Enabled: true},
	}

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	if capturedEnv == nil {
		t.Fatal("healing env not captured")
	}
	if got := capturedEnv["PLOY_API_TOKEN"]; got != "file-token-abc" {
		t.Fatalf("PLOY_API_TOKEN=%q, want file-token-abc", got)
	}
}

// TestExecuteWithHealing_SessionPropagation verifies that healing session
// artifacts are propagated across healing retries:
//  1. After a healing mod run, the session file (codex-session.txt) from /out is read.
//  2. The session is persisted to /in for subsequent attempts.
//  3. Agent-specific resume env (currently CODEX_RESUME=1 for Codex-based healers)
//     is injected into subsequent healing manifests when the image opts in.
func TestExecuteWithHealing_SessionPropagation(t *testing.T) {
	// Track healing mod container specs to verify resume env injection.
	var healerSpecs []step.ContainerSpec
	gateCallCount := 0

	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		gateCallCount++
		// Always fail gate to force multiple healing retries.
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "java", Passed: false}}, LogsText: "fail"}, nil
	}}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			// Capture healer specs for assertion.
			healerSpecs = append(healerSpecs, spec)
			return step.ContainerHandle{ID: "heal"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return []byte(""), nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	ws, err := os.MkdirTemp("", "ploy-session-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(ws) }()

	outDir, err := os.MkdirTemp("", "ploy-session-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	// Write codex-session.txt to /out before healing mod runs (simulating
	// what a session-aware healing agent would produce).
	sessionFile := filepath.Join(outDir, "codex-session.txt")
	if err := os.WriteFile(sessionFile, []byte("session-id-abc-123\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write sentinel file to test observability tracking.
	sentinelFile := filepath.Join(outDir, "request_build_validation")
	if err := os.WriteFile(sentinelFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "http://localhost", NodeID: "n"}}

	req := StartRunRequest{
		RunID:     types.RunID("t-session"),
		JobID:     types.JobID("t-job-session"),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 2, // Two retries to verify session propagation.
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "mods-codex:latest"}},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:     types.StepID(req.JobID),
		Image:  "main:latest",
		Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}},
		Gate:   &contracts.StepGateSpec{Enabled: true},
	}

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	// Verify we got 2 healing attempts (retries=2).
	if len(healerSpecs) != 2 {
		t.Fatalf("healing container calls = %d, want 2", len(healerSpecs))
	}

	// First healing attempt should NOT have CODEX_RESUME (no prior session).
	if _, hasResume := healerSpecs[0].Env["CODEX_RESUME"]; hasResume {
		t.Errorf("first healing attempt has CODEX_RESUME=%q, want absent", healerSpecs[0].Env["CODEX_RESUME"])
	}

	// Second healing attempt SHOULD have CODEX_RESUME=1 (session from first attempt).
	if healerSpecs[1].Env["CODEX_RESUME"] != "1" {
		t.Errorf("second healing attempt CODEX_RESUME=%q, want '1'", healerSpecs[1].Env["CODEX_RESUME"])
	}

	// Verify /in directory was created and contains codex-session.txt.
	if inDir == "" {
		t.Fatal("/in directory should be created for healing")
	}

	sessionInPath := filepath.Join(inDir, "codex-session.txt")
	sessionBytes, readErr := os.ReadFile(sessionInPath)
	if readErr != nil {
		t.Fatalf("failed to read codex-session.txt from /in: %v", readErr)
	}

	if got := strings.TrimSpace(string(sessionBytes)); got != "session-id-abc-123" {
		t.Errorf("codex-session.txt in /in = %q, want 'session-id-abc-123'", got)
	}
}

// TestExecuteWithHealing_NonSessionAwareHealerNoResume verifies that healing
// mods that are not marked as session-aware do not receive CODEX_RESUME even
// when a session file is available.
func TestExecuteWithHealing_NonSessionAwareHealerNoResume(t *testing.T) {
	var healerSpecs []step.ContainerSpec
	gateCallCount := 0

	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		gateCallCount++
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "java", Passed: false}}, LogsText: "fail"}, nil
	}}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			healerSpecs = append(healerSpecs, spec)
			return step.ContainerHandle{ID: "heal"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return []byte(""), nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	ws, err := os.MkdirTemp("", "ploy-nonsession-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(ws) }()

	outDir, err := os.MkdirTemp("", "ploy-nonsession-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	// Write a session file (simulating a previous agent run or external source).
	sessionFile := filepath.Join(outDir, "codex-session.txt")
	if err := os.WriteFile(sessionFile, []byte("some-session-id\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{ServerURL: "http://localhost", NodeID: "n"}}

	req := StartRunRequest{
		RunID:     types.RunID("t-nonsession"),
		JobID:     types.JobID("t-job-nonsession"),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 2,
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "standard-healer:v1"}},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:     types.StepID(req.JobID),
		Image:  "main:latest",
		Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}},
		Gate:   &contracts.StepGateSpec{Enabled: true},
	}

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	// Verify we got 2 healing attempts.
	if len(healerSpecs) != 2 {
		t.Fatalf("healing container calls = %d, want 2", len(healerSpecs))
	}

	// Neither attempt should have CODEX_RESUME (non-session-aware image).
	for i, spec := range healerSpecs {
		if _, hasResume := spec.Env["CODEX_RESUME"]; hasResume {
			t.Errorf("healing attempt %d has CODEX_RESUME=%q, want absent (non-Codex healer)", i+1, spec.Env["CODEX_RESUME"])
		}
	}
}

// TestExecuteWithHealing_DoesNotInjectBearerFromFileWhenTLSEnabled verifies that
// in TLS-enabled configurations, the bearer-token file is not used as a fallback
// for PLOY_API_TOKEN when the env var is unset.
func TestExecuteWithHealing_DoesNotInjectBearerFromFileWhenTLSEnabled(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "ploy-node-bearer-tls-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	if _, err := tmpFile.WriteString("tls-file-token\n"); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tmpFile.Name())
	t.Setenv("PLOY_API_TOKEN", "")

	mockGate := &mockGateExecutor{executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}}, LogsText: "fail"}, nil
	}}

	var capturedEnv map[string]string
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			if capturedEnv == nil {
				copied := make(map[string]string, len(spec.Env))
				for k, v := range spec.Env {
					copied[k] = v
				}
				capturedEnv = copied
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

	ws, err := os.MkdirTemp("", "ploy-tls-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(ws) }()

	outDir, _ := os.MkdirTemp("", "ploy-tls-out-*")
	defer func() { _ = os.RemoveAll(outDir) }()
	inDir := ""

	runner := step.Runner{Workspace: &mockWorkspaceHydrator{}, Containers: mockContainer, Gate: mockGate}
	rc := &runController{cfg: Config{
		ServerURL: "https://server:8443",
		NodeID:    "n",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled:  true,
				CertPath: "/etc/ploy/pki/node.crt",
				KeyPath:  "/etc/ploy/pki/node.key",
				CAPath:   "/etc/ploy/pki/ca.crt",
			},
		},
	}}

	req := StartRunRequest{
		RunID:     types.RunID("t-tls-nofallback"),
		JobID:     types.JobID("t-job-tls-nofallback"),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "heal:latest"}},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:     types.StepID(req.JobID),
		Image:  "main:latest",
		Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}},
		Gate:   &contracts.StepGateSpec{Enabled: true},
	}

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	if capturedEnv == nil {
		t.Fatal("healing env not captured")
	}
	if _, ok := capturedEnv["PLOY_API_TOKEN"]; ok {
		t.Fatalf("PLOY_API_TOKEN=%q, want unset for TLS-enabled configuration", capturedEnv["PLOY_API_TOKEN"])
	}
}

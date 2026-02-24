package nodeagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// Healing retry and edge-path tests focused on the gate-heal-regate loop.

// TestExecuteWithHealing_ModNonZeroExit_DoesNotAbort ensures a healing mig returning
// a non-zero exit code does not abort the loop; the gate is still re-run.
func TestExecuteWithHealing_ModNonZeroExit_DoesNotAbort(t *testing.T) {
	gateCallCount := 0
	mockGate := &mockGateExecutor{executeFn: func(_ context.Context, _ *contracts.StepGateSpec, _ string) (*contracts.BuildGateStageMetadata, error) {
		gateCallCount++
		passed := gateCallCount > 1
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "java", Passed: passed}}, LogsText: "gate"}, nil
	}}

	// Healing container exits with non-zero; main mig exits with zero.
	mockContainer := &mockContainerRuntime{
		createFn: func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			id := "main"
			if spec.Image == "heal:latest" {
				id = "heal"
			}
			return step.ContainerHandle(id), nil
		},
		startFn: func(_ context.Context, _ step.ContainerHandle) error { return nil },
		waitFn: func(_ context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			if string(handle) == "heal" {
				return step.ContainerResult{ExitCode: 17}, nil
			}
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(_ context.Context, _ step.ContainerHandle) ([]byte, error) { return []byte(""), nil },
		removeFn: func(_ context.Context, _ step.ContainerHandle) error { return nil },
	}

	ws, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(mockGate, mockContainer)
	rc := healingRC()
	req := healingRequest("t-nonzero", "j-nonzero", 1, "heal:latest")
	manifest := healingManifest(req)

	res, err := rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("main mig not executed successfully: exit=%d", res.ExitCode)
	}
	// With post-mig gate enabled, we now have 3 gate calls: pre-gate, pre-mig re-gate, post-mig gate.
	if gateCallCount != 3 {
		t.Fatalf("expected 3 gate calls (pre + re + post), got %d", gateCallCount)
	}
}

// TestExecuteWithHealing_RetriesValueHonored verifies that the healing retry
// limit is enforced (retries=N → exactly N healing attempts).
func TestExecuteWithHealing_RetriesValueHonored(t *testing.T) {
	gateCalls := 0
	mockGate := &mockGateExecutor{executeFn: func(_ context.Context, _ *contracts.StepGateSpec, _ string) (*contracts.BuildGateStageMetadata, error) {
		gateCalls++
		return &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "java", Passed: false}}, LogsText: "fail"}, nil
	}}

	creates := 0
	mc := noopContainer()
	mc.createFn = func(_ context.Context, _ step.ContainerSpec) (step.ContainerHandle, error) {
		creates++
		return step.ContainerHandle("heal"), nil
	}

	ws, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(mockGate, mc)
	rc := healingRC()
	req := healingRequest("t-retries", "j-retries", 2, "heal:latest")
	manifest := healingManifest(req)

	_, err := rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)
	if err == nil || !errors.Is(err, step.ErrBuildGateFailed) {
		t.Fatalf("want ErrBuildGateFailed after retries exhausted, got %v", err)
	}
	if creates != 2 {
		t.Fatalf("healing container creates=%d, want 2", creates)
	}
	if gateCalls != 3 { // 1 pre-gate + 2 re-gates
		t.Fatalf("gate calls=%d, want 3 (pre + 2 re-gates)", gateCalls)
	}
}

// TestExecuteWithHealing_HealingConfiguredNoMod_NoHealing verifies that when
// build_gate_healing is present but no mig is configured, healing is treated
// as disabled (return pre-gate failure immediately).
func TestExecuteWithHealing_HealingConfiguredNoMod_NoHealing(t *testing.T) {
	mockContainer := &mockContainerRuntime{createFn: func(_ context.Context, _ step.ContainerSpec) (step.ContainerHandle, error) {
		t.Fatalf("no container should be created when no healing mig is configured")
		return step.ContainerHandle("x"), nil
	}}

	ws, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(failingGate(), mockContainer)
	rc := healingRC()
	req := healingRequest("t-empty-strategies", "j-empty-strategies", 3, "")
	manifest := healingManifest(req)

	_, err := rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)
	if err == nil || !errors.Is(err, step.ErrBuildGateFailed) {
		t.Fatalf("want ErrBuildGateFailed without healing, got %v", err)
	}
}

// TestExecuteWithHealing_InjectsServerAndTLSVars ensures TLS and server URL
// env vars are injected into healing containers for Build Gate API access.
func TestExecuteWithHealing_InjectsServerAndTLSVars(t *testing.T) {
	mc, envPtr := envCapturingContainer()
	ws, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(failingGate(), mc)
	rc := healingRCWithConfig(Config{ServerURL: "http://127.0.0.1:8080", NodeID: "n", HTTP: HTTPConfig{TLS: TLSConfig{Enabled: false}}})
	req := healingRequest("t-tls", "j-tls", 1, "heal:latest")
	manifest := healingManifest(req)

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	envSeen := *envPtr
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
func TestExecuteWithHealing_RetriesExhausted(t *testing.T) {
	containerCreates := 0
	mc := noopContainer()
	mc.createFn = func(_ context.Context, _ step.ContainerSpec) (step.ContainerHandle, error) {
		containerCreates++
		return step.ContainerHandle("mock-container"), nil
	}

	ws, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(failingGate(), mc)
	rc := healingRC()
	req := healingRequest("test-run-exhausted", "test-job-exhausted", 2, "test/healer:latest")
	manifest := healingManifest(req)

	_, err := rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	if err == nil {
		t.Fatalf("executeWithHealing() expected error, got nil")
	}
	if !errors.Is(err, step.ErrBuildGateFailed) {
		t.Errorf("executeWithHealing() error should wrap ErrBuildGateFailed, got: %v", err)
	}
	if err.Error() != "build gate failed: healing retries exhausted" {
		t.Errorf("executeWithHealing() error = %q, want 'build gate failed: healing retries exhausted'", err.Error())
	}
	if containerCreates != 2 {
		t.Errorf("healing containers created = %d, want 2 (main mig must be skipped)", containerCreates)
	}
}

// TestExecuteWithHealing_InjectsHostWorkspaceEnv verifies that the healing
// container receives PLOY_HOST_WORKSPACE env with the host workspace path.
func TestExecuteWithHealing_InjectsHostWorkspaceEnv(t *testing.T) {
	var capturedEnv map[string]string
	var capturedMounts []step.ContainerMount
	mc := noopContainer()
	mc.createFn = func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		copied := make(map[string]string, len(spec.Env))
		for k, v := range spec.Env {
			copied[k] = v
		}
		capturedEnv = copied
		capturedMounts = append([]step.ContainerMount{}, spec.Mounts...)
		return step.ContainerHandle("heal"), nil
	}

	ws, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(failingGate(), mc)
	rc := healingRC()
	req := healingRequest("t-env", "j-env", 1, "heal:latest")
	manifest := healingManifest(req)

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	if capturedEnv == nil {
		t.Fatal("healing env not captured")
	}
	if got := capturedEnv["PLOY_HOST_WORKSPACE"]; got != ws {
		t.Fatalf("PLOY_HOST_WORKSPACE=%q, want %q", got, ws)
	}

	// Assert docker socket mount present when host socket path exists and is mountable.
	wantSock := false
	for _, m := range capturedMounts {
		if m.Target == "/var/run/docker.sock" && m.Source == "/var/run/docker.sock" {
			wantSock = true
			break
		}
	}
	if fi, err := os.Stat("/var/run/docker.sock"); err == nil && !fi.IsDir() {
		if !wantSock {
			t.Fatalf("docker.sock mount not found in healing container spec: mounts=%+v", capturedMounts)
		}
	}
}

// TestExecuteWithHealing_InjectsBearerFromEnv verifies that when PLOY_API_TOKEN
// is set in the node process environment, it is propagated into healing
// container env regardless of TLS configuration.
func TestExecuteWithHealing_InjectsBearerFromEnv(t *testing.T) {
	t.Setenv("PLOY_API_TOKEN", "env-token-123")

	mc, envPtr := envCapturingContainer()
	ws, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(failingGate(), mc)
	rc := healingRCWithConfig(Config{
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
	})
	req := healingRequest("t-env-bearer", "t-job-env-bearer", 1, "heal:latest")
	manifest := healingManifest(req)

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	envSeen := *envPtr
	if envSeen == nil {
		t.Fatal("healing env not captured")
	}
	if got := envSeen["PLOY_API_TOKEN"]; got != "env-token-123" {
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

	mc, envPtr := envCapturingContainer()
	ws, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(failingGate(), mc)
	rc := healingRCWithConfig(Config{ServerURL: "http://localhost", NodeID: "n", HTTP: HTTPConfig{TLS: TLSConfig{Enabled: false}}})
	req := healingRequest("t-file-bearer", "t-job-file-bearer", 1, "heal:latest")
	manifest := healingManifest(req)

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	envSeen := *envPtr
	if envSeen == nil {
		t.Fatal("healing env not captured")
	}
	if got := envSeen["PLOY_API_TOKEN"]; got != "file-token-abc" {
		t.Fatalf("PLOY_API_TOKEN=%q, want file-token-abc", got)
	}
}

// TestExecuteWithHealing_SessionPropagation verifies that healing session
// artifacts are propagated across healing retries.
func TestExecuteWithHealing_SessionPropagation(t *testing.T) {
	var healerSpecs []step.ContainerSpec
	mc := noopContainer()
	mc.createFn = func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		healerSpecs = append(healerSpecs, spec)
		return step.ContainerHandle("heal"), nil
	}

	ws, outDir := healingDirs(t)

	// Write codex-session.txt to /out (simulating session-aware agent output).
	if err := os.WriteFile(filepath.Join(outDir, "codex-session.txt"), []byte("session-id-abc-123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "request_build_validation"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	inDir := ""
	runner := healingRunner(failingGate(), mc)
	rc := healingRC()
	req := healingRequest("t-session", "t-job-session", 2, "migs-codex:latest")
	manifest := healingManifest(req)

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	if len(healerSpecs) != 2 {
		t.Fatalf("healing container calls = %d, want 2", len(healerSpecs))
	}
	if _, hasResume := healerSpecs[0].Env["CODEX_RESUME"]; hasResume {
		t.Errorf("first healing attempt has CODEX_RESUME=%q, want absent", healerSpecs[0].Env["CODEX_RESUME"])
	}
	if healerSpecs[1].Env["CODEX_RESUME"] != "1" {
		t.Errorf("second healing attempt CODEX_RESUME=%q, want '1'", healerSpecs[1].Env["CODEX_RESUME"])
	}
	if inDir == "" {
		t.Fatal("/in directory should be created for healing")
	}
	sessionBytes, readErr := os.ReadFile(filepath.Join(inDir, "codex-session.txt"))
	if readErr != nil {
		t.Fatalf("failed to read codex-session.txt from /in: %v", readErr)
	}
	if got := strings.TrimSpace(string(sessionBytes)); got != "session-id-abc-123" {
		t.Errorf("codex-session.txt in /in = %q, want 'session-id-abc-123'", got)
	}
}

// TestExecuteWithHealing_NonSessionAwareHealerNoResume verifies that healing
// migs that are not marked as session-aware do not receive CODEX_RESUME even
// when a session file is available.
func TestExecuteWithHealing_NonSessionAwareHealerNoResume(t *testing.T) {
	var healerSpecs []step.ContainerSpec
	mc := noopContainer()
	mc.createFn = func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		healerSpecs = append(healerSpecs, spec)
		return step.ContainerHandle("heal"), nil
	}

	ws, outDir := healingDirs(t)
	if err := os.WriteFile(filepath.Join(outDir, "codex-session.txt"), []byte("some-session-id\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	inDir := ""
	runner := healingRunner(failingGate(), mc)
	rc := healingRC()
	req := healingRequest("t-nonsession", "t-job-nonsession", 2, "standard-healer:v1")
	manifest := healingManifest(req)

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	if len(healerSpecs) != 2 {
		t.Fatalf("healing container calls = %d, want 2", len(healerSpecs))
	}
	for i, spec := range healerSpecs {
		if _, hasResume := spec.Env["CODEX_RESUME"]; hasResume {
			t.Errorf("healing attempt %d has CODEX_RESUME=%q, want absent (non-Codex healer)", i+1, spec.Env["CODEX_RESUME"])
		}
	}
}

// TestExecuteWithHealing_DoesNotInjectBearerFromFileWhenTLSEnabled verifies that
// in TLS-enabled configurations, the bearer-token file is not used as a fallback.
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

	mc, envPtr := envCapturingContainer()
	ws, outDir := healingDirs(t)
	inDir := ""
	runner := healingRunner(failingGate(), mc)
	rc := healingRCWithConfig(Config{
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
	})
	req := healingRequest("t-tls-nofallback", "t-job-tls-nofallback", 1, "heal:latest")
	manifest := healingManifest(req)

	_, _ = rc.executeWithHealing(context.Background(), runner, req, manifest, ws, outDir, &inDir, 0)

	envSeen := *envPtr
	if envSeen == nil {
		t.Fatal("healing env not captured")
	}
	if _, ok := envSeen["PLOY_API_TOKEN"]; ok {
		t.Fatalf("PLOY_API_TOKEN=%q, want unset for TLS-enabled configuration", envSeen["PLOY_API_TOKEN"])
	}
}

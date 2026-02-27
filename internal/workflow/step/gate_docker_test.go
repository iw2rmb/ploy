package step

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	units "github.com/docker/go-units"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestDockerGateExecutor_DoesNotRemoveContainerAfterExecution(t *testing.T) {
	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	workspace := createMavenWorkspace(t, "17")

	spec := &contracts.StepGateSpec{
		Enabled: true,
	}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if rt.removeCalled {
		t.Fatalf("expected Remove not to be called on container runtime after gate execution")
	}
	if !rt.createCalled || !rt.startCalled || !rt.waitCalled || !rt.logsCalled {
		t.Fatalf("expected create/start/wait/logs to be called; got %+v", rt)
	}
}

func TestDockerGateExecutor_ReportsRuntimeImageBeforeContainerCreate(t *testing.T) {
	t.Setenv("PLOY_BUILDGATE_IMAGE", "docker.io/example/gate:latest")

	var (
		observerCalled bool
		observedImage  string
	)

	rt := &testContainerRuntime{
		createFn: func(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
			if !observerCalled {
				t.Fatalf("expected runtime image observer to be called before container Create")
			}
			return ContainerHandle("mock"), nil
		},
	}
	executor := NewDockerGateExecutor(rt)

	ctx := WithGateRuntimeImageObserver(context.Background(), func(_ context.Context, image string) {
		observerCalled = true
		observedImage = image
	})

	workspace := createMavenWorkspace(t, "17")
	spec := &contracts.StepGateSpec{Enabled: true}

	if _, err := executor.Execute(ctx, spec, workspace); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !observerCalled {
		t.Fatalf("expected runtime image observer to be called")
	}
	if observedImage != "docker.io/example/gate:latest" {
		t.Fatalf("observed image = %q, want %q", observedImage, "docker.io/example/gate:latest")
	}
}

func TestDockerGateExecutor_PassesContainerLabelsFromContext(t *testing.T) {
	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	workspace := createMavenWorkspace(t, "17")
	spec := &contracts.StepGateSpec{Enabled: true}
	ctx := WithGateContainerLabels(context.Background(), map[string]string{
		types.LabelRunID: "run-123",
		types.LabelJobID: "job-456",
	})

	if _, err := executor.Execute(ctx, spec, workspace); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	if got := rt.captured.Labels[types.LabelRunID]; got != "run-123" {
		t.Fatalf("label %q = %q, want %q", types.LabelRunID, got, "run-123")
	}
	if got := rt.captured.Labels[types.LabelJobID]; got != "job-456" {
		t.Fatalf("label %q = %q, want %q", types.LabelJobID, got, "job-456")
	}
}

// TestDockerGateExecutor_EnvPassthrough verifies that environment variables from
// StepGateSpec.Env are passed through to the Docker container. This ensures that
// global env vars injected by the control plane (e.g., CA_CERTS_PEM_BUNDLE,
// CODEX_AUTH_JSON) are available to image-level startup hooks.
func TestDockerGateExecutor_EnvPassthrough(t *testing.T) {
	t.Parallel()

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	workspace := createMavenWorkspace(t, "17")

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Env: map[string]string{
			"CA_CERTS_PEM_BUNDLE": "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
			"CODEX_AUTH_JSON":     `{"token":"secret"}`,
			"CUSTOM_VAR":          "custom-value",
		},
	}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	// Verify all env vars from spec.Env are passed to the container spec.
	if rt.captured.Env == nil {
		t.Fatal("expected ContainerSpec.Env to be set, got nil")
	}
	if len(rt.captured.Env) != 3 {
		t.Fatalf("expected 3 env vars, got %d: %v", len(rt.captured.Env), rt.captured.Env)
	}

	// Check each expected key.
	expectedKeys := []string{"CA_CERTS_PEM_BUNDLE", "CODEX_AUTH_JSON", "CUSTOM_VAR"}
	for _, key := range expectedKeys {
		if _, ok := rt.captured.Env[key]; !ok {
			t.Errorf("expected env var %q to be present, but it's missing", key)
		}
	}

	// Verify values are correct.
	if rt.captured.Env["CA_CERTS_PEM_BUNDLE"] != spec.Env["CA_CERTS_PEM_BUNDLE"] {
		t.Errorf("CA_CERTS_PEM_BUNDLE mismatch: got %q, want %q",
			rt.captured.Env["CA_CERTS_PEM_BUNDLE"], spec.Env["CA_CERTS_PEM_BUNDLE"])
	}
	if rt.captured.Env["CODEX_AUTH_JSON"] != spec.Env["CODEX_AUTH_JSON"] {
		t.Errorf("CODEX_AUTH_JSON mismatch: got %q, want %q",
			rt.captured.Env["CODEX_AUTH_JSON"], spec.Env["CODEX_AUTH_JSON"])
	}
	if rt.captured.Env["CUSTOM_VAR"] != spec.Env["CUSTOM_VAR"] {
		t.Errorf("CUSTOM_VAR mismatch: got %q, want %q",
			rt.captured.Env["CUSTOM_VAR"], spec.Env["CUSTOM_VAR"])
	}
}

// TestDockerGateExecutor_EmptyEnv verifies that the gate executor handles
// empty or nil env maps gracefully without errors.
func TestDockerGateExecutor_EmptyEnv(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		env  map[string]string
	}{
		{"nil_env", nil},
		{"empty_env", map[string]string{}},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rt := &testContainerRuntime{}
			executor := NewDockerGateExecutor(rt)

			spec := &contracts.StepGateSpec{
				Enabled: true,
				Env:     tc.env,
			}

			workspace := createMavenWorkspace(t, "17")
			_, err := executor.Execute(context.Background(), spec, workspace)
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}

			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}

			// For nil/empty input, the container spec env should be nil or empty.
			if len(rt.captured.Env) != 0 {
				t.Errorf("expected empty env for %s, got %v", tc.name, rt.captured.Env)
			}
		})
	}
}

func TestDockerGateExecutor_PrepOverrideCommandPrecedence(t *testing.T) {
	t.Parallel()

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	workspace := createMavenWorkspace(t, "17")
	spec := &contracts.StepGateSpec{
		Enabled: true,
		Prep: &contracts.BuildGatePrepOverride{
			Command: contracts.CommandSpec{Shell: "echo prep-gate"},
		},
	}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	got := rt.captured.Command
	want := []string{"/bin/sh", "-c", "echo prep-gate"}
	if len(got) != len(want) {
		t.Fatalf("captured command length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("captured command[%d] = %q, want %q", i, got[i], v)
		}
	}
}

func TestDockerGateExecutor_PrepOverrideEnvPrecedence(t *testing.T) {
	t.Parallel()

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	workspace := createMavenWorkspace(t, "17")
	spec := &contracts.StepGateSpec{
		Enabled: true,
		Env: map[string]string{
			"A": "base",
			"B": "base",
		},
		Prep: &contracts.BuildGatePrepOverride{
			Command: contracts.CommandSpec{Shell: "echo prep-gate"},
			Env: map[string]string{
				"B": "prep",
				"C": "prep",
			},
		},
	}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	if got := rt.captured.Env["A"]; got != "base" {
		t.Fatalf("env[A] = %q, want %q", got, "base")
	}
	if got := rt.captured.Env["B"]; got != "prep" {
		t.Fatalf("env[B] = %q, want %q", got, "prep")
	}
	if got := rt.captured.Env["C"]; got != "prep" {
		t.Fatalf("env[C] = %q, want %q", got, "prep")
	}
}

func TestDockerGateExecutor_LimitEnvParsing(t *testing.T) {
	memHuman, err := units.RAMInBytes("1GiB")
	if err != nil {
		t.Fatalf("RAMInBytes(1GiB) error: %v", err)
	}
	diskHuman, err := units.RAMInBytes("5GiB")
	if err != nil {
		t.Fatalf("RAMInBytes(5GiB) error: %v", err)
	}

	testCases := []struct {
		name       string
		memEnv     string
		cpuEnv     string
		diskEnv    string
		wantMem    int64
		wantNano   int64
		wantDisk   int64
		wantDiskOp string
	}{
		{
			name:       "numeric_limits",
			memEnv:     "2048",
			cpuEnv:     "250",
			diskEnv:    "4096",
			wantMem:    2048,
			wantNano:   250 * 1_000_000,
			wantDisk:   4096,
			wantDiskOp: "4096",
		},
		{
			name:       "human_size_limits",
			memEnv:     "1GiB",
			cpuEnv:     "500",
			diskEnv:    "5GiB",
			wantMem:    memHuman,
			wantNano:   500 * 1_000_000,
			wantDisk:   diskHuman,
			wantDiskOp: "5GiB",
		},
		{
			name:       "invalid_values_fall_back_to_zero",
			memEnv:     "not-a-size",
			cpuEnv:     "not-a-number",
			diskEnv:    "bad-size",
			wantMem:    0,
			wantNano:   0,
			wantDisk:   0,
			wantDiskOp: "bad-size",
		},
		{
			name:       "integer_fallback_for_bytes",
			memEnv:     "-512",
			cpuEnv:     "100",
			diskEnv:    "-1234",
			wantMem:    -512,
			wantNano:   100 * 1_000_000,
			wantDisk:   -1234,
			wantDiskOp: "-1234",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(buildGateLimitMemoryEnv, tc.memEnv)
			t.Setenv(buildGateLimitCPUEnv, tc.cpuEnv)
			t.Setenv(buildGateLimitDiskEnv, tc.diskEnv)

			rt := &testContainerRuntime{}
			executor := NewDockerGateExecutor(rt)
			workspace := createMavenWorkspace(t, "17")

			spec := &contracts.StepGateSpec{Enabled: true}
			if _, err := executor.Execute(context.Background(), spec, workspace); err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}
			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}

			if rt.captured.LimitMemoryBytes != tc.wantMem {
				t.Fatalf("LimitMemoryBytes=%d, want %d", rt.captured.LimitMemoryBytes, tc.wantMem)
			}
			if rt.captured.LimitNanoCPUs != tc.wantNano {
				t.Fatalf("LimitNanoCPUs=%d, want %d", rt.captured.LimitNanoCPUs, tc.wantNano)
			}
			if rt.captured.LimitDiskBytes != tc.wantDisk {
				t.Fatalf("LimitDiskBytes=%d, want %d", rt.captured.LimitDiskBytes, tc.wantDisk)
			}
			if rt.captured.StorageSizeOpt != tc.wantDiskOp {
				t.Fatalf("StorageSizeOpt=%q, want %q", rt.captured.StorageSizeOpt, tc.wantDiskOp)
			}
		})
	}
}

func TestDockerGateExecutor_GradleCommandOmitsFailFast(t *testing.T) {
	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	tmpDir := createGradleWorkspace(t, "17")

	spec := &contracts.StepGateSpec{
		Enabled: true,
	}

	if _, err := executor.Execute(context.Background(), spec, tmpDir); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	if len(rt.captured.Command) != 3 {
		t.Fatalf("expected 3-element command, got %v", rt.captured.Command)
	}

	cmd := rt.captured.Command[2]
	if !strings.Contains(cmd, "gradle -q --stacktrace --build-cache") {
		t.Fatalf("expected gradle command with -q --stacktrace --build-cache, got %q", cmd)
	}
	if strings.Contains(cmd, "--fail-fast") {
		t.Fatalf("expected gradle command not to contain --fail-fast, got %q", cmd)
	}
	if !strings.Contains(cmd, "test -p /workspace") {
		t.Fatalf("expected gradle command to run tests in /workspace, got %q", cmd)
	}
}

// TestDockerGateExecutor_CAPreambleIncluded verifies that the CA bundle preamble
// is prepended to Maven, Gradle, and plain Java build commands. This ensures that
// CA_CERTS_PEM_BUNDLE from global config is consumed by the gate container for
// trusting corporate proxies and private registries.
func TestDockerGateExecutor_CAPreambleIncluded(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		workspace   func(t *testing.T) string
		spec        func() *contracts.StepGateSpec
		expectInCmd string // substring expected in the shell command
	}{
		{
			name:        "maven",
			workspace:   func(t *testing.T) string { return createMavenWorkspace(t, "17") },
			spec:        func() *contracts.StepGateSpec { return &contracts.StepGateSpec{Enabled: true} },
			expectInCmd: "mvn --ff -B -q -e",
		},
		{
			name:        "gradle",
			workspace:   func(t *testing.T) string { return createGradleWorkspace(t, "17") },
			spec:        func() *contracts.StepGateSpec { return &contracts.StepGateSpec{Enabled: true} },
			expectInCmd: "gradle -q --stacktrace",
		},
		{
			name:        "go",
			workspace:   func(t *testing.T) string { return createGoWorkspace(t, "1.22") },
			spec:        func() *contracts.StepGateSpec { return &contracts.StepGateSpec{Enabled: true} },
			expectInCmd: "go test ./...",
		},
		{
			name:        "cargo",
			workspace:   func(t *testing.T) string { return createCargoWorkspace(t, "1.76") },
			spec:        func() *contracts.StepGateSpec { return &contracts.StepGateSpec{Enabled: true} },
			expectInCmd: "cargo test",
		},
		{
			name:      "pip",
			workspace: func(t *testing.T) string { return createPythonWorkspace(t, "3.11") },
			spec: func() *contracts.StepGateSpec {
				return &contracts.StepGateSpec{
					Enabled: true,
					ImageOverrides: []contracts.BuildGateImageRule{{
						Stack: contracts.StackExpectation{Language: "python", Release: "3.11"},
						Image: "python:3.11",
					}},
				}
			},
			expectInCmd: "python -m compileall",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rt := &testContainerRuntime{}
			executor := NewDockerGateExecutor(rt)

			tmpDir := tc.workspace(t)
			spec := tc.spec()
			_, err := executor.Execute(context.Background(), spec, tmpDir)
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}

			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}
			if len(rt.captured.Command) != 3 {
				t.Fatalf("expected 3-element command, got %v", rt.captured.Command)
			}

			cmd := rt.captured.Command[2]

			// Verify CA preamble is present.
			if !strings.Contains(cmd, "CA_CERTS_PEM_BUNDLE") {
				t.Errorf("expected CA_CERTS_PEM_BUNDLE in command, got %q", cmd)
			}
			if !strings.Contains(cmd, "update-ca-certificates") {
				t.Errorf("expected update-ca-certificates in command, got %q", cmd)
			}
			if !strings.Contains(cmd, "keytool -importcert") {
				t.Errorf("expected keytool -importcert in command, got %q", cmd)
			}
			if !strings.Contains(cmd, "ploy-gate") {
				t.Errorf("expected ploy-gate CA directory name in command, got %q", cmd)
			}

			// Verify the build command is still present after the preamble.
			if !strings.Contains(cmd, tc.expectInCmd) {
				t.Errorf("expected %q in command after preamble, got %q", tc.expectInCmd, cmd)
			}
		})
	}
}

// TestCAPreambleScript verifies the caPreambleScript function returns a valid
// shell script that handles CA bundle installation.
func TestCAPreambleScript(t *testing.T) {
	t.Parallel()

	preamble := caPreambleScript()

	// Verify key components are present.
	expectedFragments := []string{
		"CA_CERTS_PEM_BUNDLE",              // env var check
		"mktemp",                           // temp file creation
		"awk",                              // cert splitting
		"update-ca-certificates",           // system CA update
		"keytool -importcert",              // Java cacerts import
		"ploy_gate_pem_",                   // alias prefix
		"changeit",                         // default keystore password
		"--- CA bundle injection preamble", // start marker
		"--- End CA bundle preamble",       // end marker
	}

	for _, fragment := range expectedFragments {
		if !strings.Contains(preamble, fragment) {
			t.Errorf("expected %q in CA preamble, got:\n%s", fragment, preamble)
		}
	}
}

// --- Stack Gate Pre-Check Tests ---

// createTestMappingFile creates a temporary build gate image mapping file for tests.
// Returns the path to the file. The file is automatically cleaned up when the test completes.
func createTestMappingFile(t *testing.T, rules string) string {
	t.Helper()
	tmpDir := t.TempDir()
	mappingPath := filepath.Join(tmpDir, "build-gate-images.yaml")
	content := `BuildGateImages:
` + rules
	if err := os.WriteFile(mappingPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to create mapping file: %v", err)
	}
	return mappingPath
}

// createMavenWorkspace creates a workspace with a valid Maven pom.xml that has Java version.
func createMavenWorkspace(t *testing.T, javaVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>test</groupId>
  <artifactId>test</artifactId>
  <version>1.0</version>
  <properties>
    <maven.compiler.release>` + javaVersion + `</maven.compiler.release>
  </properties>
</project>`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomContent), 0o644); err != nil {
		t.Fatalf("failed to create pom.xml: %v", err)
	}
	return tmpDir
}

// createMavenWorkspaceNoJavaVersion creates a workspace with pom.xml but no Java release configured.
func createMavenWorkspaceNoJavaVersion(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>test</groupId>
  <artifactId>test</artifactId>
  <version>1.0</version>
</project>`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomContent), 0o644); err != nil {
		t.Fatalf("failed to create pom.xml: %v", err)
	}
	return tmpDir
}

// createGradleWorkspace creates a workspace with a valid Gradle build.gradle that has Java version.
func createGradleWorkspace(t *testing.T, javaVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	gradleContent := `plugins {
    id 'java'
}

java {
    sourceCompatibility = JavaVersion.VERSION_` + javaVersion + `
    targetCompatibility = JavaVersion.VERSION_` + javaVersion + `
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(gradleContent), 0o644); err != nil {
		t.Fatalf("failed to create build.gradle: %v", err)
	}
	return tmpDir
}

func createGoWorkspace(t *testing.T, goVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	goModuleFile := "go." + "mo" + "d"
	goMod := "module example.com/test\n\ngo " + goVersion + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, goModuleFile), []byte(goMod), 0o644); err != nil {
		t.Fatalf("failed to create go module file: %v", err)
	}
	return tmpDir
}

func createCargoWorkspace(t *testing.T, rustVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	cargo := `[package]
name = "test"
version = "0.1.0"
edition = "2021"
rust-version = "` + rustVersion + `"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte(cargo), 0o644); err != nil {
		t.Fatalf("failed to create Cargo.toml: %v", err)
	}
	return tmpDir
}

func createPythonWorkspace(t *testing.T, pythonVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".python-version"), []byte(pythonVersion+"\n"), 0o644); err != nil {
		t.Fatalf("failed to create .python-version: %v", err)
	}
	return tmpDir
}

// TestGateDocker_StackGate_PreCheckPass verifies that Stack Gate pre-check passes
// when detected stack matches expectations, and container is executed.
func TestGateDocker_StackGate_PreCheckPass(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspace(t, "17")

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		ImageOverrides: []contracts.BuildGateImageRule{{
			Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			Image: "maven:3-eclipse-temurin-17",
		}},
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify container was executed.
	if !rt.createCalled {
		t.Error("expected container Create to be called")
	}
	if !rt.startCalled {
		t.Error("expected container Start to be called")
	}

	// Verify RuntimeImage is set.
	if meta.StackGate.RuntimeImage != "maven:3-eclipse-temurin-17" {
		t.Errorf("RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, "maven:3-eclipse-temurin-17")
	}

	// Verify stack gate result.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "pass" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "pass")
	}
	if meta.StackGate.Enabled != true {
		t.Errorf("StackGate.Enabled = %v, want true", meta.StackGate.Enabled)
	}
	if meta.StackGate.Expected == nil || meta.StackGate.Expected.Release != "17" {
		t.Errorf("StackGate.Expected.Release = %v, want 17", meta.StackGate.Expected)
	}
	if meta.StackGate.Detected == nil || meta.StackGate.Detected.Release != "17" {
		t.Errorf("StackGate.Detected.Release = %v, want 17", meta.StackGate.Detected)
	}
}

// TestGateDocker_StackGate_PreCheckMismatch verifies that Stack Gate fails early
// when detected stack doesn't match expectations, without running container.
func TestGateDocker_StackGate_PreCheckMismatch(t *testing.T) {
	t.Parallel()
	expectedRuntimeImage := resolveContainerRegistryPrefix() + "/maven:3-eclipse-temurin-17"

	// Create workspace with Java 11, but expect Java 17.
	workspace := createMavenWorkspace(t, "11")

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify container was NOT executed (early return).
	if rt.createCalled {
		t.Error("expected container Create NOT to be called on mismatch")
	}

	// Verify stack gate result.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "mismatch" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "mismatch")
	}
	if !strings.Contains(meta.StackGate.Reason, "release:") {
		t.Errorf("StackGate.Reason = %q, expected to contain 'release:'", meta.StackGate.Reason)
	}

	// Verify runtime image is still resolved for observability (even though the container is not run).
	if meta.RuntimeImage != expectedRuntimeImage {
		t.Errorf("RuntimeImage = %q, want %q", meta.RuntimeImage, expectedRuntimeImage)
	}
	if meta.StackGate.RuntimeImage != expectedRuntimeImage {
		t.Errorf("StackGate.RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, expectedRuntimeImage)
	}

	// Verify static check reports failure.
	if len(meta.StaticChecks) == 0 || meta.StaticChecks[0].Passed {
		t.Errorf("expected static check to report failure, got %+v", meta.StaticChecks)
	}
	if len(meta.StaticChecks) > 0 {
		if meta.StaticChecks[0].Tool != "stack-gate" {
			t.Errorf("StaticChecks[0].Tool = %q, want %q", meta.StaticChecks[0].Tool, "stack-gate")
		}
		if meta.StaticChecks[0].Language != "java" {
			t.Errorf("StaticChecks[0].Language = %q, want %q", meta.StaticChecks[0].Language, "java")
		}
	}

	// Verify log finding with STACK_GATE_MISMATCH code.
	found := false
	for _, f := range meta.LogFindings {
		if f.Code == "STACK_GATE_MISMATCH" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log finding with code STACK_GATE_MISMATCH, got %+v", meta.LogFindings)
	}
}

// TestGateDocker_StackGate_PreCheckUnknown_Ambiguous verifies that Stack Gate fails early
// when both Maven and Gradle files are present (ambiguous detection).
func TestGateDocker_StackGate_PreCheckUnknown_Ambiguous(t *testing.T) {
	t.Parallel()
	expectedRuntimeImage := resolveContainerRegistryPrefix() + "/maven:3-eclipse-temurin-17"

	// Create workspace with both pom.xml and build.gradle.
	tmpDir := t.TempDir()
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>test</groupId>
  <artifactId>test</artifactId>
  <version>1.0</version>
  <properties>
    <maven.compiler.release>17</maven.compiler.release>
  </properties>
</project>`
	gradleContent := `plugins { id 'java' }
java { toolchain { languageVersion = JavaLanguageVersion.of(17) } }`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomContent), 0o644); err != nil {
		t.Fatalf("failed to create pom.xml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(gradleContent), 0o644); err != nil {
		t.Fatalf("failed to create build.gradle: %v", err)
	}

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, tmpDir)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify container was NOT executed.
	if rt.createCalled {
		t.Error("expected container Create NOT to be called on ambiguous")
	}

	// Verify stack gate result is unknown.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "unknown" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "unknown")
	}
	if !strings.Contains(meta.StackGate.Reason, "ambiguous") {
		t.Errorf("StackGate.Reason = %q, expected to contain 'ambiguous'", meta.StackGate.Reason)
	}

	// Verify runtime image is still resolved for observability (even though the container is not run).
	if meta.RuntimeImage != expectedRuntimeImage {
		t.Errorf("RuntimeImage = %q, want %q", meta.RuntimeImage, expectedRuntimeImage)
	}
	if meta.StackGate.RuntimeImage != expectedRuntimeImage {
		t.Errorf("StackGate.RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, expectedRuntimeImage)
	}

	// Verify log finding with STACK_GATE_UNKNOWN code.
	found := false
	for _, f := range meta.LogFindings {
		if f.Code == "STACK_GATE_UNKNOWN" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log finding with code STACK_GATE_UNKNOWN, got %+v", meta.LogFindings)
	}
	if len(meta.StaticChecks) > 0 {
		if meta.StaticChecks[0].Tool != "stack-gate" {
			t.Errorf("StaticChecks[0].Tool = %q, want %q", meta.StaticChecks[0].Tool, "stack-gate")
		}
		if meta.StaticChecks[0].Language != "java" {
			t.Errorf("StaticChecks[0].Language = %q, want %q", meta.StaticChecks[0].Language, "java")
		}
	}
}

// TestGateDocker_StackGate_PreCheckUnknown_NoFiles verifies that Stack Gate fails early
// when no Maven or Gradle build files are present.
func TestGateDocker_StackGate_PreCheckUnknown_NoFiles(t *testing.T) {
	t.Parallel()
	expectedRuntimeImage := resolveContainerRegistryPrefix() + "/maven:3-eclipse-temurin-17"

	// Empty workspace.
	tmpDir := t.TempDir()

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, tmpDir)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify container was NOT executed.
	if rt.createCalled {
		t.Error("expected container Create NOT to be called on unknown")
	}

	// Verify stack gate result is unknown.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "unknown" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "unknown")
	}

	// Verify runtime image is still resolved for observability (even though the container is not run).
	if meta.RuntimeImage != expectedRuntimeImage {
		t.Errorf("RuntimeImage = %q, want %q", meta.RuntimeImage, expectedRuntimeImage)
	}
	if meta.StackGate.RuntimeImage != expectedRuntimeImage {
		t.Errorf("StackGate.RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, expectedRuntimeImage)
	}
	if len(meta.StaticChecks) > 0 {
		if meta.StaticChecks[0].Tool != "stack-gate" {
			t.Errorf("StaticChecks[0].Tool = %q, want %q", meta.StaticChecks[0].Tool, "stack-gate")
		}
		if meta.StaticChecks[0].Language != "java" {
			t.Errorf("StaticChecks[0].Language = %q, want %q", meta.StaticChecks[0].Language, "java")
		}
	}
}

// TestGateDocker_StackGate_ImageResolution verifies correct image is used when Stack Gate matches.
// This test uses ImageOverrides to provide inline rules and verifies the complete resolver flow.
func TestGateDocker_StackGate_ImageResolution(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspace(t, "17")

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		ImageOverrides: []contracts.BuildGateImageRule{{
			Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			Image: "custom-maven:java17",
		}},
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify correct image was used.
	if rt.captured.Image != "custom-maven:java17" {
		t.Errorf("Image = %q, want %q", rt.captured.Image, "custom-maven:java17")
	}

	// Verify RuntimeImage is set in metadata.
	if meta.StackGate.RuntimeImage != "custom-maven:java17" {
		t.Errorf("RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, "custom-maven:java17")
	}

	// Verify stack gate result shows pass.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "pass" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "pass")
	}
}

func TestGateDocker_StackGate_NoMatchingDefaultRule_ReturnsNoImageRule(t *testing.T) {
	t.Parallel()

	workspace := createPythonWorkspace(t, "3.11")

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "python",
				Tool:     "pip",
				Release:  "3.11",
			},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	// Should return metadata, not error.
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Container should NOT be executed.
	if rt.createCalled {
		t.Error("expected container Create NOT to be called")
	}

	// Verify stack gate result is unknown.
	if meta.StackGate == nil {
		t.Fatal("expected StackGate result in metadata")
	}
	if meta.StackGate.Result != "unknown" {
		t.Errorf("StackGate.Result = %q, want %q", meta.StackGate.Result, "unknown")
	}

	// Verify log finding with appropriate code.
	found := false
	for _, f := range meta.LogFindings {
		if f.Code == "STACK_GATE_NO_IMAGE_RULE" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log finding with code STACK_GATE_NO_IMAGE_RULE, got %+v", meta.LogFindings)
	}
}

func TestGateDocker_StackGate_UsesDefaultMappingFileByDefault(t *testing.T) {
	t.Parallel()
	expectedRuntimeImage := resolveContainerRegistryPrefix() + "/maven:3-eclipse-temurin-17"

	workspace := createMavenWorkspace(t, "17")

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if rt.captured.Image != expectedRuntimeImage {
		t.Errorf("Image = %q, want %q", rt.captured.Image, expectedRuntimeImage)
	}
	if meta.StackGate == nil || meta.StackGate.RuntimeImage != expectedRuntimeImage {
		t.Fatalf("RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, expectedRuntimeImage)
	}
}

// TestGateDocker_StackGate_UsesBuildgateImageEnv verifies that PLOY_BUILDGATE_IMAGE
// is honored in Stack Gate mode.
func TestGateDocker_StackGate_UsesBuildgateImageEnv(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv.

	workspace := createMavenWorkspace(t, "17")

	t.Setenv("PLOY_BUILDGATE_IMAGE", "override:latest")

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		ImageOverrides: []contracts.BuildGateImageRule{{
			Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			Image: "resolved-from-mapping:17",
		}},
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if rt.captured.Image != "override:latest" {
		t.Errorf("Image = %q, want %q", rt.captured.Image, "override:latest")
	}

	// Verify RuntimeImage in metadata.
	if meta.StackGate.RuntimeImage != "override:latest" {
		t.Errorf("RuntimeImage = %q, want %q", meta.StackGate.RuntimeImage, "override:latest")
	}
}

func TestGateDocker_StackDetect_DefaultTrue_FallsBackOnMissingVersion(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspaceNoJavaVersion(t)

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		// Provide inline mapping for the fallback expectation.
		ImageOverrides: []contracts.BuildGateImageRule{{
			Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			Image: "custom-maven:java17",
		}},
		StackDetect: &contracts.BuildGateStackConfig{
			Enabled:  true,
			Language: "java",
			Release:  "17",
			Default:  true,
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Container should be executed.
	if !rt.createCalled {
		t.Fatal("expected container Create to be called")
	}
	if rt.captured.Image != "custom-maven:java17" {
		t.Fatalf("Image = %q, want %q", rt.captured.Image, "custom-maven:java17")
	}
	if meta == nil || len(meta.StaticChecks) == 0 {
		t.Fatal("expected non-empty metadata")
	}
	if meta.StaticChecks[0].Tool != "maven" {
		t.Fatalf("tool = %q, want %q", meta.StaticChecks[0].Tool, "maven")
	}
}

func TestGateDocker_StackDetect_DefaultFalse_CancelsOnDetectionFailure(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspaceNoJavaVersion(t)

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackDetect: &contracts.BuildGateStackConfig{
			Enabled:  true,
			Language: "java",
			Release:  "17",
			Default:  false,
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrRepoCancelled) {
		t.Fatalf("error = %v, want ErrRepoCancelled", err)
	}

	// Container must NOT be executed.
	if rt.createCalled {
		t.Fatal("expected container Create NOT to be called")
	}
	if meta == nil || len(meta.LogFindings) == 0 {
		t.Fatal("expected log findings in metadata")
	}
}

package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// mockGateRuntimeMinimal implements the subset of ContainerRuntime used by
// dockerGateExecutor plus Remove, so we can verify cleanup behavior without
// depending on the real Docker client or runner mocks.
type mockGateRuntimeMinimal struct {
	createCalled bool
	startCalled  bool
	waitCalled   bool
	logsCalled   bool
	removeCalled bool
	lastSpec     ContainerSpec
}

func (m *mockGateRuntimeMinimal) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	m.createCalled = true
	m.lastSpec = spec
	return ContainerHandle{ID: "mock-id"}, nil
}

func (m *mockGateRuntimeMinimal) Start(ctx context.Context, h ContainerHandle) error {
	m.startCalled = true
	return nil
}

func (m *mockGateRuntimeMinimal) Wait(ctx context.Context, h ContainerHandle) (ContainerResult, error) {
	m.waitCalled = true
	return ContainerResult{ExitCode: 0}, nil
}

func (m *mockGateRuntimeMinimal) Logs(ctx context.Context, h ContainerHandle) ([]byte, error) {
	m.logsCalled = true
	return []byte("ok"), nil
}

func (m *mockGateRuntimeMinimal) Remove(ctx context.Context, h ContainerHandle) error {
	m.removeCalled = true
	return nil
}

func TestDockerGateExecutor_RemovesContainerAfterExecution(t *testing.T) {
	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: "java",
	}

	_, err := executor.Execute(context.Background(), spec, "/tmp/workspace")
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.removeCalled {
		t.Fatalf("expected Remove to be called on container runtime after gate execution")
	}
	if !rt.createCalled || !rt.startCalled || !rt.waitCalled || !rt.logsCalled {
		t.Fatalf("expected create/start/wait/logs to be called before remove; got %+v", rt)
	}
}

// TestDockerGateExecutor_EnvPassthrough verifies that environment variables from
// StepGateSpec.Env are passed through to the Docker container. This ensures that
// global env vars injected by the control plane (e.g., CA_CERTS_PEM_BUNDLE,
// CODEX_AUTH_JSON) are available to image-level startup hooks.
func TestDockerGateExecutor_EnvPassthrough(t *testing.T) {
	t.Parallel()

	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: "java",
		Env: map[string]string{
			"CA_CERTS_PEM_BUNDLE": "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
			"CODEX_AUTH_JSON":     `{"token":"secret"}`,
			"CUSTOM_VAR":          "custom-value",
		},
	}

	_, err := executor.Execute(context.Background(), spec, t.TempDir())
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	// Verify all env vars from spec.Env are passed to the container spec.
	if rt.lastSpec.Env == nil {
		t.Fatal("expected ContainerSpec.Env to be set, got nil")
	}
	if len(rt.lastSpec.Env) != 3 {
		t.Fatalf("expected 3 env vars, got %d: %v", len(rt.lastSpec.Env), rt.lastSpec.Env)
	}

	// Check each expected key.
	expectedKeys := []string{"CA_CERTS_PEM_BUNDLE", "CODEX_AUTH_JSON", "CUSTOM_VAR"}
	for _, key := range expectedKeys {
		if _, ok := rt.lastSpec.Env[key]; !ok {
			t.Errorf("expected env var %q to be present, but it's missing", key)
		}
	}

	// Verify values are correct.
	if rt.lastSpec.Env["CA_CERTS_PEM_BUNDLE"] != spec.Env["CA_CERTS_PEM_BUNDLE"] {
		t.Errorf("CA_CERTS_PEM_BUNDLE mismatch: got %q, want %q",
			rt.lastSpec.Env["CA_CERTS_PEM_BUNDLE"], spec.Env["CA_CERTS_PEM_BUNDLE"])
	}
	if rt.lastSpec.Env["CODEX_AUTH_JSON"] != spec.Env["CODEX_AUTH_JSON"] {
		t.Errorf("CODEX_AUTH_JSON mismatch: got %q, want %q",
			rt.lastSpec.Env["CODEX_AUTH_JSON"], spec.Env["CODEX_AUTH_JSON"])
	}
	if rt.lastSpec.Env["CUSTOM_VAR"] != spec.Env["CUSTOM_VAR"] {
		t.Errorf("CUSTOM_VAR mismatch: got %q, want %q",
			rt.lastSpec.Env["CUSTOM_VAR"], spec.Env["CUSTOM_VAR"])
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

			rt := &mockGateRuntimeMinimal{}
			executor := NewDockerGateExecutor(rt)

			spec := &contracts.StepGateSpec{
				Enabled: true,
				Profile: "java",
				Env:     tc.env,
			}

			_, err := executor.Execute(context.Background(), spec, t.TempDir())
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}

			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}

			// For nil/empty input, the container spec env should be nil or empty.
			if len(rt.lastSpec.Env) != 0 {
				t.Errorf("expected empty env for %s, got %v", tc.name, rt.lastSpec.Env)
			}
		})
	}
}

func TestDockerGateExecutor_GradleCommandOmitsFailFast(t *testing.T) {
	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	// Use an explicit java-gradle profile; workspace contents are irrelevant for this path.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte{}, 0o644); err != nil {
		t.Fatalf("failed to create dummy build.gradle: %v", err)
	}

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: "java-gradle",
	}

	if _, err := executor.Execute(context.Background(), spec, tmpDir); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	if len(rt.lastSpec.Command) != 3 {
		t.Fatalf("expected 3-element command, got %v", rt.lastSpec.Command)
	}

	cmd := rt.lastSpec.Command[2]
	if !strings.Contains(cmd, "gradle -q --stacktrace") {
		t.Fatalf("expected gradle command with -q --stacktrace, got %q", cmd)
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
		profile     string
		setupFile   string // file to create in workspace to trigger detection
		expectInCmd string // substring expected in the shell command
	}{
		{
			name:        "maven_profile",
			profile:     "java-maven",
			setupFile:   "pom.xml",
			expectInCmd: "mvn --ff -B -q -e",
		},
		{
			name:        "gradle_profile",
			profile:     "java-gradle",
			setupFile:   "build.gradle",
			expectInCmd: "gradle -q --stacktrace",
		},
		{
			name:        "java_profile",
			profile:     "java",
			setupFile:   "", // no build file needed for plain java
			expectInCmd: "javac --release 17",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rt := &mockGateRuntimeMinimal{}
			executor := NewDockerGateExecutor(rt)

			tmpDir := t.TempDir()
			if tc.setupFile != "" {
				if err := os.WriteFile(filepath.Join(tmpDir, tc.setupFile), []byte{}, 0o644); err != nil {
					t.Fatalf("failed to create %s: %v", tc.setupFile, err)
				}
			}

			spec := &contracts.StepGateSpec{
				Enabled: true,
				Profile: tc.profile,
			}

			_, err := executor.Execute(context.Background(), spec, tmpDir)
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}

			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}
			if len(rt.lastSpec.Command) != 3 {
				t.Fatalf("expected 3-element command, got %v", rt.lastSpec.Command)
			}

			cmd := rt.lastSpec.Command[2]

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
	content := `images:
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

// createGradleWorkspace creates a workspace with a valid Gradle build.gradle that has Java version.
func createGradleWorkspace(t *testing.T, javaVersion string) string {
	t.Helper()
	tmpDir := t.TempDir()
	gradleContent := `plugins {
    id 'java'
}

java {
    toolchain {
        languageVersion = JavaLanguageVersion.of(` + javaVersion + `)
    }
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(gradleContent), 0o644); err != nil {
		t.Fatalf("failed to create build.gradle: %v", err)
	}
	return tmpDir
}

// TestGateDocker_StackGate_PreCheckPass verifies that Stack Gate pre-check passes
// when detected stack matches expectations, and container is executed.
func TestGateDocker_StackGate_PreCheckPass(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspace(t, "17")

	rt := &mockGateRuntimeMinimal{}
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
			ImageOverrides: []contracts.BuildGateImageRule{{
				Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
				Image: "maven:3-eclipse-temurin-17",
			}},
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

	// Create workspace with Java 11, but expect Java 17.
	workspace := createMavenWorkspace(t, "11")

	rt := &mockGateRuntimeMinimal{}
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

	// Verify static check reports failure.
	if len(meta.StaticChecks) == 0 || meta.StaticChecks[0].Passed {
		t.Errorf("expected static check to report failure, got %+v", meta.StaticChecks)
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

	rt := &mockGateRuntimeMinimal{}
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
}

// TestGateDocker_StackGate_PreCheckUnknown_NoFiles verifies that Stack Gate fails early
// when no Maven or Gradle build files are present.
func TestGateDocker_StackGate_PreCheckUnknown_NoFiles(t *testing.T) {
	t.Parallel()

	// Empty workspace.
	tmpDir := t.TempDir()

	rt := &mockGateRuntimeMinimal{}
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
}

// TestGateDocker_StackGate_ImageResolution verifies correct image is used when Stack Gate matches.
// This test uses ImageOverrides to provide inline rules and verifies the complete resolver flow.
func TestGateDocker_StackGate_ImageResolution(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspace(t, "17")

	rt := &mockGateRuntimeMinimal{}
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
			ImageOverrides: []contracts.BuildGateImageRule{{
				Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
				Image: "custom-maven:java17",
			}},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify correct image was used.
	if rt.lastSpec.Image != "custom-maven:java17" {
		t.Errorf("Image = %q, want %q", rt.lastSpec.Image, "custom-maven:java17")
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

// TestGateDocker_StackGate_ImageResolutionFailure verifies metadata with Result="unknown"
// when no matching image rule exists. Container should NOT be executed.
func TestGateDocker_StackGate_ImageResolutionFailure(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspace(t, "17")

	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	// Stack Gate expects Java 17 but no matching rules.
	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
			// Non-matching rule: Java 11 instead of 17
			ImageOverrides: []contracts.BuildGateImageRule{{
				Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "11"},
				Image: "maven:java11",
			}},
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

// TestGateDocker_StackGate_NoDefaults_Maven verifies that Stack Gate mode does NOT
// fall back to default Maven image when no rules match. Returns metadata with Result="unknown".
func TestGateDocker_StackGate_NoDefaults_Maven(t *testing.T) {
	t.Parallel()

	workspace := createMavenWorkspace(t, "17")

	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	// Stack Gate enabled but no matching rules - should return metadata with Result="unknown".
	// With empty ImageOverrides and no default file, we get STACK_GATE_IMAGE_MAPPING_ERROR.
	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
			ImageOverrides: []contracts.BuildGateImageRule{}, // Empty - no rules.
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	// Should return metadata, not error.
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify container was NOT created.
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

	// Verify log finding with appropriate code (either image mapping error or no rule error).
	found := false
	for _, f := range meta.LogFindings {
		if f.Code == "STACK_GATE_IMAGE_MAPPING_ERROR" || f.Code == "STACK_GATE_NO_IMAGE_RULE" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log finding with image mapping error code, got %+v", meta.LogFindings)
	}
}

// TestGateDocker_StackGate_NoDefaults_Gradle verifies that Stack Gate mode does NOT
// fall back to default Gradle image when no rules match. Returns metadata with Result="unknown".
func TestGateDocker_StackGate_NoDefaults_Gradle(t *testing.T) {
	t.Parallel()

	workspace := createGradleWorkspace(t, "17")

	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	// Stack Gate enabled but no matching rules - should return metadata with Result="unknown".
	// With empty ImageOverrides and no default file, we get STACK_GATE_IMAGE_MAPPING_ERROR.
	spec := &contracts.StepGateSpec{
		Enabled: true,
		StackGate: &contracts.StepGateStackSpec{
			Enabled: true,
			Expect: &contracts.StackExpectation{
				Language: "java",
				Tool:     "gradle",
				Release:  "17",
			},
			ImageOverrides: []contracts.BuildGateImageRule{}, // Empty - no rules.
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	// Should return metadata, not error.
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify container was NOT created.
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

	// Verify log finding with appropriate code (either image mapping error or no rule error).
	found := false
	for _, f := range meta.LogFindings {
		if f.Code == "STACK_GATE_IMAGE_MAPPING_ERROR" || f.Code == "STACK_GATE_NO_IMAGE_RULE" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log finding with image mapping error code, got %+v", meta.LogFindings)
	}
}

// TestGateDocker_StackGate_IgnoresBuildgateImageEnv verifies that PLOY_BUILDGATE_IMAGE
// environment variable is ignored in Stack Gate mode - images are always resolved via mapping.
func TestGateDocker_StackGate_IgnoresBuildgateImageEnv(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv.

	workspace := createMavenWorkspace(t, "17")

	// Set env var that SHOULD be ignored in Stack Gate mode.
	t.Setenv("PLOY_BUILDGATE_IMAGE", "should-be-ignored:latest")

	rt := &mockGateRuntimeMinimal{}
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
			ImageOverrides: []contracts.BuildGateImageRule{{
				Stack: contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
				Image: "resolved-from-mapping:17",
			}},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verify the resolved image was used, NOT the env var.
	if rt.lastSpec.Image != "resolved-from-mapping:17" {
		t.Errorf("Image = %q, want %q (should ignore PLOY_BUILDGATE_IMAGE)",
			rt.lastSpec.Image, "resolved-from-mapping:17")
	}

	// Verify RuntimeImage in metadata.
	if meta.StackGate.RuntimeImage != "resolved-from-mapping:17" {
		t.Errorf("RuntimeImage = %q, want %q",
			meta.StackGate.RuntimeImage, "resolved-from-mapping:17")
	}
}

package nodeagent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
)

func TestNormalizeSBOMRuntimeRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		release string
		want    string
	}{
		{name: "empty falls back to jdk17", release: "", want: sbomReleaseJDK17},
		{name: "whitespace falls back to jdk17", release: "   ", want: sbomReleaseJDK17},
		{name: "unsupported falls back to jdk17", release: "21", want: sbomReleaseJDK17},
		{name: "jdk11 accepted", release: sbomReleaseJDK11, want: sbomReleaseJDK11},
		{name: "jdk17 accepted", release: sbomReleaseJDK17, want: sbomReleaseJDK17},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeSBOMRuntimeRelease(tc.release); got != tc.want {
				t.Fatalf("normalizeSBOMRuntimeRelease(%q)=%q, want %q", tc.release, got, tc.want)
			}
		})
	}
}

func TestSBOMRuntimeTagForRelease(t *testing.T) {
	t.Parallel()

	if got := sbomRuntimeTagForRelease(sbomReleaseJDK11); got != "jdk11" {
		t.Fatalf("sbomRuntimeTagForRelease(jdk11)=%q, want jdk11", got)
	}
	if got := sbomRuntimeTagForRelease(sbomReleaseJDK17); got != "jdk17" {
		t.Fatalf("sbomRuntimeTagForRelease(jdk17)=%q, want jdk17", got)
	}
	if got := sbomRuntimeTagForRelease("unknown"); got != "jdk17" {
		t.Fatalf("sbomRuntimeTagForRelease(unknown)=%q, want jdk17", got)
	}
}

func TestResolveSBOMRuntimeStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		stack contracts.MigStack
		want  contracts.MigStack
	}{
		{name: "maven", stack: contracts.MigStackJavaMaven, want: contracts.MigStackJavaMaven},
		{name: "gradle", stack: contracts.MigStackJavaGradle, want: contracts.MigStackJavaGradle},
		{name: "java uses maven runtime", stack: contracts.MigStackJava, want: contracts.MigStackJavaMaven},
		{name: "unknown uses maven runtime", stack: contracts.MigStackUnknown, want: contracts.MigStackJavaMaven},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveSBOMRuntimeStack(tc.stack); got != tc.want {
				t.Fatalf("resolveSBOMRuntimeStack(%q)=%q, want %q", tc.stack, got, tc.want)
			}
		})
	}
}

func TestApplySBOMRuntimeForStack_ConfiguresManifest(t *testing.T) {
	t.Setenv(sbomImageRegistryEnvKey, "ghcr.io/acme")

	tests := []struct {
		name              string
		stack             contracts.MigStack
		release           string
		wantImage         string
		wantRuntimeStack  contracts.MigStack
		wantShellSnippets []string
	}{
		{
			name:             "maven",
			stack:            contracts.MigStackJavaMaven,
			release:          sbomReleaseJDK17,
			wantImage:        "ghcr.io/acme/sbom-maven:jdk17",
			wantRuntimeStack: contracts.MigStackJavaMaven,
			wantShellSnippets: []string{
				"missing /workspace/pom.xml",
				sbomMavenCollectorScript,
			},
		},
		{
			name:             "gradle",
			stack:            contracts.MigStackJavaGradle,
			release:          sbomReleaseJDK11,
			wantImage:        "ghcr.io/acme/sbom-gradle:jdk11",
			wantRuntimeStack: contracts.MigStackJavaGradle,
			wantShellSnippets: []string{
				`PLOY_SBOM_GRADLE_CMD="/workspace/gradlew"`,
				`PLOY_SBOM_GRADLE_CMD="gradle"`,
				sbomGradleCollectorScript,
			},
		},
		{
			name:             "unknown fallback collector path",
			stack:            contracts.MigStackUnknown,
			release:          "unknown",
			wantImage:        "ghcr.io/acme/sbom-maven:jdk17",
			wantRuntimeStack: contracts.MigStackJavaMaven,
			wantShellSnippets: []string{
				sbomMavenCollectorScript,
				sbomGradleCollectorScript,
				`PLOY_SBOM_GRADLE_CMD="/workspace/gradlew"`,
				`PLOY_SBOM_GRADLE_CMD="gradle"`,
				"gradle build detected but no gradle wrapper and no gradle binary available",
				"unable to resolve sbom collector",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			manifest := &contracts.StepManifest{}
			if err := applySBOMRuntimeForStack(manifest, tc.stack, tc.release); err != nil {
				t.Fatalf("applySBOMRuntimeForStack: %v", err)
			}
			if got := manifest.Image; got != tc.wantImage {
				t.Fatalf("manifest.Image=%q, want %q", got, tc.wantImage)
			}
			if got := manifest.Envs["PLOY_SBOM_STACK"]; got != string(tc.wantRuntimeStack) {
				t.Fatalf("manifest.Envs[PLOY_SBOM_STACK]=%q, want %q", got, tc.wantRuntimeStack)
			}
			if got := manifest.Envs[contracts.PLOYStackLanguageEnv]; got != "java" {
				t.Fatalf("manifest.Envs[%s]=%q, want java", contracts.PLOYStackLanguageEnv, got)
			}
			wantTool := ""
			switch tc.wantRuntimeStack {
			case contracts.MigStackJavaGradle:
				wantTool = "gradle"
			default:
				wantTool = "maven"
			}
			if got := manifest.Envs[contracts.PLOYStackToolEnv]; got != wantTool {
				t.Fatalf("manifest.Envs[%s]=%q, want %q", contracts.PLOYStackToolEnv, got, wantTool)
			}
			wantRelease := normalizeSBOMRuntimeRelease(tc.release)
			if got := manifest.Envs[contracts.PLOYStackReleaseEnv]; got != wantRelease {
				t.Fatalf("manifest.Envs[%s]=%q, want %q", contracts.PLOYStackReleaseEnv, got, wantRelease)
			}
			if len(manifest.Command) < 3 {
				t.Fatalf("manifest.Command=%v, want shell command", manifest.Command)
			}
			shell := manifest.Command[len(manifest.Command)-1]
			for _, wantSnippet := range tc.wantShellSnippets {
				if !strings.Contains(shell, wantSnippet) {
					t.Fatalf("shell command missing %q: %q", wantSnippet, shell)
				}
			}
			if strings.Contains(shell, "classpath_init") {
				t.Fatalf("shell command must not use inline init script after extraction: %q", shell)
			}
			if strings.Contains(shell, "ployWriteJavaClasspath") {
				t.Fatalf("shell command must delegate classpath task wiring to script files: %q", shell)
			}
			if tc.stack == contracts.MigStackUnknown && strings.Contains(shell, ": > /out/"+sbomDependencyOutputFileName) {
				t.Fatalf("unknown stack command uses placeholder output write: %q", shell)
			}
			if got := manifest.Envs["PLOY_SBOM_DEPENDENCY_OUTPUT"]; got != "/out/"+sbomDependencyOutputFileName {
				t.Fatalf("manifest.Envs[PLOY_SBOM_DEPENDENCY_OUTPUT]=%q, want %q", got, "/out/"+sbomDependencyOutputFileName)
			}
			if got := manifest.Envs["PLOY_SBOM_JAVA_CLASSPATH_OUTPUT"]; got != "/out/"+sbomJavaClasspathFileName {
				t.Fatalf("manifest.Envs[PLOY_SBOM_JAVA_CLASSPATH_OUTPUT]=%q, want %q", got, "/out/"+sbomJavaClasspathFileName)
			}
		})
	}
}

func TestDetectSBOMStackFromWorkspace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		layout    map[string]string // relative path -> content; empty map = empty dir
		wantStack contracts.MigStack
		wantErr   bool
		wantAmbig bool
	}{
		{
			name:    "empty workspace",
			layout:  map[string]string{},
			wantErr: true,
		},
		{
			name:      "maven pom",
			layout:    map[string]string{"pom.xml": "<project/>"},
			wantStack: contracts.MigStackJavaMaven,
		},
		{
			name:      "gradle kts",
			layout:    map[string]string{"build.gradle.kts": "plugins {}"},
			wantStack: contracts.MigStackJavaGradle,
		},
		{
			name: "ambiguous pom + gradle",
			layout: map[string]string{
				"pom.xml":      "<project/>",
				"build.gradle": "plugins {}",
			},
			wantErr:   true,
			wantAmbig: true,
		},
		{
			name:    "settings.gradle.kts only",
			layout:  map[string]string{"settings.gradle.kts": `rootProject.name = "x"`},
			wantErr: true,
		},
		{
			name:    "gradlew only",
			layout:  map[string]string{"gradlew": "#!/bin/sh"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for name, content := range tc.layout {
				p := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			got, err := detectSBOMStackFromWorkspace(dir)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got stack=%q", got)
				}
				if tc.wantAmbig {
					var detErr *stackdetect.DetectionError
					if !errors.As(err, &detErr) || !detErr.IsAmbiguous() {
						t.Fatalf("expected ambiguous DetectionError, got %v", err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantStack {
				t.Errorf("stack = %q, want %q", got, tc.wantStack)
			}
		})
	}
}

func TestDetectSBOMStackFromWorkspace_UnsupportedTool(t *testing.T) {
	t.Parallel()

	goWorkspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(goWorkspace, "go.mod"), []byte("module example.com/x\ngo 1.24"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if _, err := detectSBOMStackFromWorkspace(goWorkspace); err == nil {
		t.Fatal("go workspace: expected unsupported-tool error, got nil")
	} else if !strings.Contains(err.Error(), "unsupported sbom tool") {
		t.Fatalf("go workspace: expected unsupported-tool error, got %v", err)
	}
}

func TestBuildSBOMManifest_IncludesPhaseCA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cycleName string
		wantCA    []string
	}{
		{name: "pre gate cycle uses pre phase CA", cycleName: preGateCycleName, wantCA: []string{"ca-pre"}},
		{name: "post gate cycle uses post phase CA", cycleName: postGateCycleName, wantCA: []string{"ca-post"}},
		{name: "re gate cycle uses post phase CA", cycleName: "re-gate-1", wantCA: []string{"ca-post"}},
		{name: "unknown cycle has no phase CA", cycleName: "other-cycle", wantCA: nil},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := StartRunRequest{
				RunID:     "run-123",
				JobID:     "job-123",
				RepoURL:   "https://example.com/repo.git",
				BaseRef:   "main",
				TargetRef: "main",
				TypedOptions: RunOptions{
					BuildGate: BuildGateOptions{
						Pre: &contracts.BuildGatePhaseConfig{CA: []string{"ca-pre"}},
						Post: &contracts.BuildGatePhaseConfig{
							CA: []string{"ca-post"},
						},
					},
				},
			}

			manifest, err := buildSBOMManifest(req, tc.cycleName, contracts.MigStackJavaMaven)
			if err != nil {
				t.Fatalf("buildSBOMManifest: %v", err)
			}
			if len(manifest.CA) != len(tc.wantCA) {
				t.Fatalf("manifest.CA len=%d, want %d (%v)", len(manifest.CA), len(tc.wantCA), manifest.CA)
			}
			for i := range tc.wantCA {
				if manifest.CA[i] != tc.wantCA[i] {
					t.Fatalf("manifest.CA[%d]=%q, want %q", i, manifest.CA[i], tc.wantCA[i])
				}
			}
		})
	}
}

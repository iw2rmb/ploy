package nodeagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

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
		name               string
		stack              contracts.MigStack
		wantImage          string
		wantRuntimeStack   contracts.MigStack
		wantCommandSnippet string
	}{
		{
			name:               "maven",
			stack:              contracts.MigStackJavaMaven,
			wantImage:          "ghcr.io/acme/sbom-maven:latest",
			wantRuntimeStack:   contracts.MigStackJavaMaven,
			wantCommandSnippet: "mvn -B -q -f /workspace/pom.xml",
		},
		{
			name:               "gradle",
			stack:              contracts.MigStackJavaGradle,
			wantImage:          "ghcr.io/acme/sbom-gradle:latest",
			wantRuntimeStack:   contracts.MigStackJavaGradle,
			wantCommandSnippet: "gradle -q -p /workspace dependencies",
		},
		{
			name:               "unknown fallback collector path",
			stack:              contracts.MigStackUnknown,
			wantImage:          "ghcr.io/acme/sbom-maven:latest",
			wantRuntimeStack:   contracts.MigStackJavaMaven,
			wantCommandSnippet: "unable to resolve sbom collector",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			manifest := &contracts.StepManifest{}
			if err := applySBOMRuntimeForStack(manifest, tc.stack); err != nil {
				t.Fatalf("applySBOMRuntimeForStack: %v", err)
			}
			if got := manifest.Image; got != tc.wantImage {
				t.Fatalf("manifest.Image=%q, want %q", got, tc.wantImage)
			}
			if got := manifest.Envs["PLOY_SBOM_STACK"]; got != string(tc.wantRuntimeStack) {
				t.Fatalf("manifest.Envs[PLOY_SBOM_STACK]=%q, want %q", got, tc.wantRuntimeStack)
			}
			if len(manifest.Command) < 3 {
				t.Fatalf("manifest.Command=%v, want shell command", manifest.Command)
			}
			shell := manifest.Command[len(manifest.Command)-1]
			if !strings.Contains(shell, tc.wantCommandSnippet) {
				t.Fatalf("shell command missing %q: %q", tc.wantCommandSnippet, shell)
			}
			if tc.stack == contracts.MigStackUnknown && strings.Contains(shell, ": > /out/"+sbomDependencyOutputFileName) {
				t.Fatalf("unknown stack command uses placeholder output write: %q", shell)
			}
		})
	}
}

func TestDetectSBOMStackFromWorkspace(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	if got := detectSBOMStackFromWorkspace(workspace, contracts.MigStackUnknown); got != contracts.MigStackUnknown {
		t.Fatalf("empty workspace detection=%q, want %q", got, contracts.MigStackUnknown)
	}

	mavenWorkspace := filepath.Join(workspace, "maven")
	if err := os.MkdirAll(mavenWorkspace, 0o755); err != nil {
		t.Fatalf("mkdir maven workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mavenWorkspace, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatalf("write pom.xml: %v", err)
	}
	if got := detectSBOMStackFromWorkspace(mavenWorkspace, contracts.MigStackUnknown); got != contracts.MigStackJavaMaven {
		t.Fatalf("maven workspace detection=%q, want %q", got, contracts.MigStackJavaMaven)
	}

	gradleWorkspace := filepath.Join(workspace, "gradle")
	if err := os.MkdirAll(gradleWorkspace, 0o755); err != nil {
		t.Fatalf("mkdir gradle workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gradleWorkspace, "build.gradle.kts"), []byte("plugins {}"), 0o644); err != nil {
		t.Fatalf("write build.gradle.kts: %v", err)
	}
	if got := detectSBOMStackFromWorkspace(gradleWorkspace, contracts.MigStackUnknown); got != contracts.MigStackJavaGradle {
		t.Fatalf("gradle workspace detection=%q, want %q", got, contracts.MigStackJavaGradle)
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

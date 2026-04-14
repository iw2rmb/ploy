package nodeagent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	iversion "github.com/iw2rmb/ploy/internal/version"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
)

func TestSBOMRuntimeImageTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "empty falls back to latest", version: "", want: "latest"},
		{name: "whitespace falls back to latest", version: "   ", want: "latest"},
		{name: "dev falls back to latest", version: "dev", want: "latest"},
		{name: "uppercase dev falls back to latest", version: "DEV", want: "latest"},
		{name: "semver uses runtime version", version: "v0.1.7", want: "v0.1.7"},
		{name: "prerelease uses runtime version", version: "v1.2.3-rc.1", want: "v1.2.3-rc.1"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sbomRuntimeImageTag(tc.version); got != tc.want {
				t.Fatalf("sbomRuntimeImageTag(%q)=%q, want %q", tc.version, got, tc.want)
			}
		})
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
	tag := sbomRuntimeImageTag(iversion.Version)

	tests := []struct {
		name               string
		stack              contracts.MigStack
		wantImage          string
		wantRuntimeStack   contracts.MigStack
		wantCommandSnippet string
		wantExtraSnippet   string
	}{
		{
			name:               "maven",
			stack:              contracts.MigStackJavaMaven,
			wantImage:          "ghcr.io/acme/sbom-maven:" + tag,
			wantRuntimeStack:   contracts.MigStackJavaMaven,
			wantCommandSnippet: "mvn -B -q -f /workspace/pom.xml",
		},
		{
			name:               "gradle",
			stack:              contracts.MigStackJavaGradle,
			wantImage:          "ghcr.io/acme/sbom-gradle:" + tag,
			wantRuntimeStack:   contracts.MigStackJavaGradle,
			wantCommandSnippet: "-q -p /workspace dependencies",
			wantExtraSnippet:   "buildEnvironment",
		},
		{
			name:               "unknown fallback collector path",
			stack:              contracts.MigStackUnknown,
			wantImage:          "ghcr.io/acme/sbom-maven:" + tag,
			wantRuntimeStack:   contracts.MigStackJavaMaven,
			wantCommandSnippet: "unable to resolve sbom collector",
			wantExtraSnippet:   "buildEnvironment",
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
			if tc.wantExtraSnippet != "" && !strings.Contains(shell, tc.wantExtraSnippet) {
				t.Fatalf("shell command missing %q: %q", tc.wantExtraSnippet, shell)
			}
			if tc.stack != contracts.MigStackJavaMaven && !strings.Contains(shell, "ployWriteJavaClasspath") {
				t.Fatalf("shell command missing ployWriteJavaClasspath task invocation: %q", shell)
			}
			if strings.Contains(shell, "classpath_init") || strings.Contains(shell, ` -I "`) {
				t.Fatalf("shell command unexpectedly contains inline init-script injection: %q", shell)
			}
			if tc.stack == contracts.MigStackUnknown && strings.Contains(shell, ": > /out/"+sbomDependencyOutputFileName) {
				t.Fatalf("unknown stack command uses placeholder output write: %q", shell)
			}
		})
	}
}

func TestDetectSBOMStackFromWorkspace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		layout      map[string]string // relative path -> content; empty map = empty dir
		wantStack   contracts.MigStack
		wantErr     bool
		wantAmbig   bool
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

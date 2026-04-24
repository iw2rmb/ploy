package nodeagent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSBOMClasspathCollectors_IncludeTestDependenciesAndOutputs(t *testing.T) {
	t.Parallel()

	repoRoot := repoRootForNodeagentTests(t)
	type setupResult struct {
		extraEnv []string
		want     []string
		wantNot  []string
	}
	tests := []struct {
		name      string
		scriptRel string
		setup     func(t *testing.T, workspace, binDir string) setupResult
	}{
		{
			name:      "maven collector includes test scope and test outputs",
			scriptRel: "images/sbom/shared/collect-java-classpath-maven.sh",
			setup: func(t *testing.T, workspace, binDir string) setupResult {
				t.Helper()

				pomPath := filepath.Join(workspace, "pom.xml")
				if err := os.WriteFile(pomPath, []byte("<project/>"), 0o644); err != nil {
					t.Fatalf("write pom.xml: %v", err)
				}
				want := []string{
					"/deps/compile.jar",
					"/deps/runtime.jar",
					"/deps/test.jar",
					filepath.Join(workspace, "target", "classes"),
					filepath.Join(workspace, "target", "resources"),
					filepath.Join(workspace, "target", "test-classes"),
					filepath.Join(workspace, "target", "test-resources"),
				}
				for _, dir := range want[3:] {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatalf("mkdir %s: %v", dir, err)
					}
				}

				mvnPath := filepath.Join(binDir, "mvn")
				mvnStub := `#!/usr/bin/env bash
set -euo pipefail

output=""
scope=""
for arg in "$@"; do
  case "$arg" in
    -Dmdep.outputFile=*) output="${arg#*=}" ;;
    -DincludeScope=*) scope="${arg#*=}" ;;
  esac
done

if [[ "$*" == *"dependency:build-classpath"* ]]; then
  case "$scope" in
    compile) printf '/deps/compile.jar\n' > "$output" ;;
    runtime) printf '/deps/runtime.jar' > "$output" ;;
    test) printf '/deps/test.jar\n' > "$output" ;;
    *) : > "$output" ;;
  esac
  exit 0
fi

if [[ "$*" == *"dependency:list"* ]]; then
  echo "org.example:demo:jar:1.0.0:compile"
  exit 0
fi

exit 0
`
				if err := writeExecutableScript(mvnPath, mvnStub); err != nil {
					t.Fatalf("write mvn stub: %v", err)
				}
				return setupResult{
					want:    want,
					wantNot: []string{"/deps/runtime.jar/deps/test.jar"},
				}
			},
		},
		{
			name:      "gradle collector includes test classpaths and test outputs",
			scriptRel: "images/sbom/shared/collect-java-classpath-gradle.sh",
			setup: func(t *testing.T, workspace, binDir string) setupResult {
				t.Helper()

				mainClasses := filepath.Join(workspace, "build", "classes", "java", "main")
				mainResources := filepath.Join(workspace, "build", "resources", "main")
				testClasses := filepath.Join(workspace, "build", "classes", "java", "test")
				testResources := filepath.Join(workspace, "build", "resources", "test")
				for _, dir := range []string{mainClasses, mainResources, testClasses, testResources} {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatalf("mkdir %s: %v", dir, err)
					}
				}

				classpathStub := strings.Join([]string{
					"/deps/compile.jar",
					"/deps/runtime.jar",
					"/deps/test-compile.jar",
					"/deps/test-runtime.jar",
					mainClasses,
					mainResources,
					testClasses,
					testResources,
				}, "\n") + "\n"
				gradlewPath := filepath.Join(workspace, "gradlew")
				gradlewStub := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

if [[ "$*" == *"ployWriteJavaClasspath"* ]]; then
  output="${PLOY_SBOM_JAVA_CLASSPATH_OUTPUT:-/share/java.classpath}"
  cat > "$output" <<'EOF'
%sEOF
  exit 0
fi

echo "ok"
exit 0
`, classpathStub)
				if err := writeExecutableScript(gradlewPath, gradlewStub); err != nil {
					t.Fatalf("write gradlew stub: %v", err)
				}

				initScript := filepath.Join(workspace, "sbom.init.gradle")
				if err := os.WriteFile(initScript, []byte("// stub"), 0o644); err != nil {
					t.Fatalf("write gradle init stub: %v", err)
				}

				return setupResult{
					extraEnv: []string{"PLOY_SBOM_GRADLE_INIT_SCRIPT=" + initScript},
					want: []string{
						"/deps/test-compile.jar",
						"/deps/test-runtime.jar",
						testClasses,
						testResources,
					},
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			workspace := filepath.Join(tmpDir, "workspace")
			outDir := filepath.Join(tmpDir, "out")
			binDir := filepath.Join(tmpDir, "bin")
			for _, dir := range []string{workspace, outDir, binDir} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}

			setup := tc.setup(t, workspace, binDir)
			classpathPath := filepath.Join(outDir, sbomJavaClasspathFileName)
			rawOutputPath := filepath.Join(outDir, sbomDependencyOutputFileName)
			cmd := exec.Command("bash", filepath.Join(repoRoot, tc.scriptRel))
			cmd.Env = append(os.Environ(),
				"PATH="+binDir+":"+os.Getenv("PATH"),
				"PLOY_SBOM_WORKSPACE="+workspace,
				"PLOY_SBOM_DEPENDENCY_OUTPUT="+rawOutputPath,
				"PLOY_SBOM_JAVA_CLASSPATH_OUTPUT="+classpathPath,
			)
			cmd.Env = append(cmd.Env, setup.extraEnv...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run %s: %v\n%s", tc.scriptRel, err, string(out))
			}

			gotClasspath, err := os.ReadFile(classpathPath)
			if err != nil {
				t.Fatalf("read classpath output: %v", err)
			}
			for _, want := range setup.want {
				if !strings.Contains(string(gotClasspath), want+"\n") && !strings.HasSuffix(string(gotClasspath), want) {
					t.Fatalf("classpath output missing %q:\n%s", want, string(gotClasspath))
				}
			}
			for _, wantNot := range setup.wantNot {
				if strings.Contains(string(gotClasspath), wantNot) {
					t.Fatalf("classpath output unexpectedly contains %q:\n%s", wantNot, string(gotClasspath))
				}
			}
		})
	}
}

func TestGradleClasspathInitScripts_IncludeTestConfigurations(t *testing.T) {
	t.Parallel()

	repoRoot := repoRootForNodeagentTests(t)
	tests := []struct {
		name     string
		fileRel  string
		snippets []string
	}{
		{
			name:    "shared sbom init script includes test configs and test source set",
			fileRel: "images/sbom/shared/gradle-write-java-classpath.init.gradle",
			snippets: []string{
				"testCompileClasspath",
				"testRuntimeClasspath",
				"findByName('test')",
			},
		},
		{
			name:    "cache init script includes test configs and test source set",
			fileRel: "images/gates/gradle/cache.init.gradle",
			snippets: []string{
				"testCompileClasspath",
				"testRuntimeClasspath",
				"findByName(\"test\")",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(filepath.Join(repoRoot, tc.fileRel))
			if err != nil {
				t.Fatalf("read %s: %v", tc.fileRel, err)
			}
			text := string(raw)
			for _, snippet := range tc.snippets {
				if !strings.Contains(text, snippet) {
					t.Fatalf("%s missing %q", tc.fileRel, snippet)
				}
			}
		})
	}
}

func writeExecutableScript(path string, body string) error {
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		return err
	}
	return nil
}

func repoRootForNodeagentTests(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

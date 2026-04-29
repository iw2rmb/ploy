package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func resolveORWCLIScript(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	candidates := []string{
		filepath.Join(root, "images", "orw", "orw-cli-java-17-maven", "orw-cli.sh"),
		filepath.Join(root, "images", "orw", "orw-cli-java-17-gradle", "orw-cli.sh"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	t.Fatalf("orw-cli script not found in expected locations: %v", candidates)
	return ""
}

func TestOrwCLI_AppliesWithStandaloneCLIAndNoBuildToolInvocation(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	binDir := t.TempDir()

	srcPath := filepath.Join(workspace, "App.java")
	if err := os.WriteFile(srcPath, []byte("class App {}\n"), 0o644); err != nil {
		t.Fatalf("write App.java: %v", err)
	}

	rewritePath := filepath.Join(binDir, "rewrite")
	rewriteScript := `#!/usr/bin/env bash
set -euo pipefail
workspace=""
classpath_file=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) workspace="${2:-}"; shift 2 ;;
    --classpath-file) classpath_file="${2:-}"; shift 2 ;;
    *) shift ;;
  esac
done
echo "[rewrite-stub] apply invoked"
echo "[rewrite-stub] classpath=$classpath_file"
if [[ "$classpath_file" != "/share/java.classpath" ]]; then
  echo "[rewrite-stub] unexpected classpath file" >&2
  exit 68
fi
if [[ -n "$workspace" ]]; then
  printf '// rewritten by rewrite-stub\n' >> "$workspace/App.java"
fi
`
	if err := os.WriteFile(rewritePath, []byte(rewriteScript), 0o755); err != nil {
		t.Fatalf("write rewrite stub: %v", err)
	}

	mvnPath := filepath.Join(binDir, "mvn")
	mvnScript := `#!/usr/bin/env bash
echo "[mvn-stub] should not run" >&2
exit 66
`
	if err := os.WriteFile(mvnPath, []byte(mvnScript), 0o755); err != nil {
		t.Fatalf("write mvn stub: %v", err)
	}

	gradlePath := filepath.Join(binDir, "gradle")
	gradleScript := `#!/usr/bin/env bash
echo "[gradle-stub] should not run" >&2
exit 67
`
	if err := os.WriteFile(gradlePath, []byte(gradleScript), 0o755); err != nil {
		t.Fatalf("write gradle stub: %v", err)
	}

	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP=org.openrewrite.recipe",
		"RECIPE_ARTIFACT=rewrite-migrate-java",
		"RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17",
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orw-cli failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	updatedSrc, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read App.java: %v", err)
	}
	if !strings.Contains(string(updatedSrc), "rewritten by rewrite-stub") {
		t.Fatalf("expected rewrite stub change in workspace, got:\n%s", string(updatedSrc))
	}

	logBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLITransformLogName))
	if err != nil {
		t.Fatalf("read transform.log: %v", err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "[rewrite-stub] apply invoked") {
		t.Fatalf("transform.log missing rewrite invocation marker:\n%s", logText)
	}
	if !strings.Contains(logText, "[rewrite-stub] classpath=/share/java.classpath") {
		t.Fatalf("transform.log missing required classpath-file argument:\n%s", logText)
	}
	if strings.Contains(logText, "[mvn-stub] should not run") || strings.Contains(logText, "[gradle-stub] should not run") {
		t.Fatalf("transform.log shows build-tool invocation:\n%s", logText)
	}
	if !strings.Contains(logText, "[orw-cli] Coords: org.openrewrite.recipe:rewrite-migrate-java") {
		t.Fatalf("transform.log missing recipe coords:\n%s", logText)
	}
	if strings.Contains(logText, "rewriteRun") || strings.Contains(logText, "rewrite-maven-plugin:run") {
		t.Fatalf("transform.log shows plugin-coupled execution path:\n%s", logText)
	}

	reportBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLIReportFileName))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	report, err := contracts.ParseORWCLIReport(reportBytes)
	if err != nil {
		t.Fatalf("parse report.json: %v\nreport=%s", err, string(reportBytes))
	}
	if !report.Success {
		t.Fatalf("report.success=false, expected true: %s", string(reportBytes))
	}
}

func TestOrwCLI_PrefersOutRewriteConfigAndActivatesYamlName(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	binDir := t.TempDir()

	rewriteYAML := "type: specs.openrewrite.org/v1beta/recipe\nname: PloyApplyYaml\nrecipeList: []\n"
	if err := os.WriteFile(filepath.Join(outdir, "rewrite.yml"), []byte(rewriteYAML), 0o644); err != nil {
		t.Fatalf("write out rewrite.yml: %v", err)
	}

	rewritePath := filepath.Join(binDir, "rewrite")
	rewriteScript := `#!/usr/bin/env bash
set -euo pipefail
config=""
recipe=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --config) config="${2:-}"; shift 2 ;;
    --recipe) recipe="${2:-}"; shift 2 ;;
    *) shift ;;
  esac
done
echo "[rewrite-stub] config=$config"
echo "[rewrite-stub] recipe=$recipe"
if [[ "$config" != "${OUT_EXPECTED_CONFIG:-}" ]]; then
  echo "[rewrite-stub] unexpected config path" >&2
  exit 51
fi
if [[ "$recipe" != "PloyApplyYaml" ]]; then
  echo "[rewrite-stub] unexpected recipe name" >&2
  exit 52
fi
`
	if err := os.WriteFile(rewritePath, []byte(rewriteScript), 0o755); err != nil {
		t.Fatalf("write rewrite stub: %v", err)
	}

	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP=org.openrewrite.recipe",
		"RECIPE_ARTIFACT=rewrite-migrate-java",
		"RECIPE_VERSION=3.20.0",
		"RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17",
		"OUT_EXPECTED_CONFIG="+filepath.Join(outdir, "rewrite.yml"),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orw-cli failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	logBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLITransformLogName))
	if err != nil {
		t.Fatalf("read transform.log: %v", err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "[rewrite-stub] config="+filepath.Join(outdir, "rewrite.yml")) {
		t.Fatalf("transform.log missing /out rewrite.yml usage:\n%s", logText)
	}
	if !strings.Contains(logText, "[rewrite-stub] recipe=PloyApplyYaml") {
		t.Fatalf("transform.log missing recipe name from /out rewrite.yml:\n%s", logText)
	}
}

func TestOrwCLI_DoesNotAutoUseWorkspaceRewriteConfig(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	binDir := t.TempDir()

	rewriteYAML := "type: specs.openrewrite.org/v1beta/recipe\nname: WorkspaceRecipe\nrecipeList: []\n"
	if err := os.WriteFile(filepath.Join(workspace, "rewrite.yml"), []byte(rewriteYAML), 0o644); err != nil {
		t.Fatalf("write workspace rewrite.yml: %v", err)
	}

	rewritePath := filepath.Join(binDir, "rewrite")
	rewriteScript := `#!/usr/bin/env bash
set -euo pipefail
config=""
recipe=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --config) config="${2:-}"; shift 2 ;;
    --recipe) recipe="${2:-}"; shift 2 ;;
    *) shift ;;
  esac
done
echo "[rewrite-stub] config=$config"
echo "[rewrite-stub] recipe=$recipe"
if [[ -n "$config" ]]; then
  echo "[rewrite-stub] unexpected config path" >&2
  exit 53
fi
if [[ "$recipe" != "org.openrewrite.java.migrate.UpgradeToJava17" ]]; then
  echo "[rewrite-stub] unexpected recipe name" >&2
  exit 54
fi
`
	if err := os.WriteFile(rewritePath, []byte(rewriteScript), 0o755); err != nil {
		t.Fatalf("write rewrite stub: %v", err)
	}

	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP=org.openrewrite.recipe",
		"RECIPE_ARTIFACT=rewrite-migrate-java",
		"RECIPE_VERSION=3.20.0",
		"RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17",
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orw-cli failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	logBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLITransformLogName))
	if err != nil {
		t.Fatalf("read transform.log: %v", err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "[rewrite-stub] config=") {
		t.Fatalf("transform.log missing config marker:\n%s", logText)
	}
	if strings.Contains(logText, "[rewrite-stub] config="+filepath.Join(workspace, "rewrite.yml")) {
		t.Fatalf("transform.log unexpectedly used workspace rewrite.yml:\n%s", logText)
	}
	if !strings.Contains(logText, "[rewrite-stub] recipe=org.openrewrite.java.migrate.UpgradeToJava17") {
		t.Fatalf("transform.log missing class recipe fallback:\n%s", logText)
	}
}

func TestOrwCLI_UsesYamlDefaultsWhenRecipeEnvMissing(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	binDir := t.TempDir()

	rewriteYAML := "type: specs.openrewrite.org/v1beta/recipe\nname: PloyApplyYaml\nrecipeList: []\n"
	if err := os.WriteFile(filepath.Join(outdir, "rewrite.yml"), []byte(rewriteYAML), 0o644); err != nil {
		t.Fatalf("write out rewrite.yml: %v", err)
	}

	rewritePath := filepath.Join(binDir, "rewrite")
	rewriteScript := `#!/usr/bin/env bash
set -euo pipefail
config=""
recipe=""
coords=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --config) config="${2:-}"; shift 2 ;;
    --recipe) recipe="${2:-}"; shift 2 ;;
    --coords) coords="${2:-}"; shift 2 ;;
    *) shift ;;
  esac
done
echo "[rewrite-stub] config=$config"
echo "[rewrite-stub] recipe=$recipe"
echo "[rewrite-stub] coords=$coords"
`
	if err := os.WriteFile(rewritePath, []byte(rewriteScript), 0o755); err != nil {
		t.Fatalf("write rewrite stub: %v", err)
	}

	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orw-cli failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	logBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLITransformLogName))
	if err != nil {
		t.Fatalf("read transform.log: %v", err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "[rewrite-stub] config="+filepath.Join(outdir, "rewrite.yml")) {
		t.Fatalf("transform.log missing /out rewrite.yml usage:\n%s", logText)
	}
	if !strings.Contains(logText, "[rewrite-stub] recipe=PloyApplyYaml") {
		t.Fatalf("transform.log missing recipe name from /out rewrite.yml:\n%s", logText)
	}
	if !strings.Contains(logText, "[rewrite-stub] coords=org.openrewrite:rewrite-java") {
		t.Fatalf("transform.log missing YAML-mode default coords:\n%s", logText)
	}
	if !strings.Contains(logText, "Applied YAML-mode default recipe coordinates/classname") {
		t.Fatalf("transform.log missing YAML-mode defaults marker:\n%s", logText)
	}
}

func TestOrwCLI_RejectsMissingRecipeEnvWithoutYamlConfig(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected orw-cli to fail when recipe env is missing outside YAML mode")
	}

	reportBytes, readErr := os.ReadFile(filepath.Join(outdir, contracts.ORWCLIReportFileName))
	if readErr != nil {
		t.Fatalf("read report.json: %v", readErr)
	}
	report, parseErr := contracts.ParseORWCLIReport(reportBytes)
	if parseErr != nil {
		t.Fatalf("parse report.json: %v\nreport=%s", parseErr, string(reportBytes))
	}
	if report.Success {
		t.Fatalf("report.success=true, expected false: %s", string(reportBytes))
	}
	if report.ErrorKind != contracts.ORWCLIErrorKindInput {
		t.Fatalf("error_kind=%q, want %q", report.ErrorKind, contracts.ORWCLIErrorKindInput)
	}
	if !strings.Contains(report.Message, "RECIPE_GROUP/RECIPE_ARTIFACT/RECIPE_CLASSNAME are required") {
		t.Fatalf("unexpected report message: %q", report.Message)
	}
}

func TestOrwCLI_UnsupportedTypeAttributionWritesDeterministicReport(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	binDir := t.TempDir()

	rewritePath := filepath.Join(binDir, "rewrite")
	rewriteScript := `#!/usr/bin/env bash
set -euo pipefail
echo "type-attribution-unavailable: cannot resolve classpath" >&2
exit 17
`
	if err := os.WriteFile(rewritePath, []byte(rewriteScript), 0o755); err != nil {
		t.Fatalf("write rewrite stub: %v", err)
	}

	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP=org.openrewrite.recipe",
		"RECIPE_ARTIFACT=rewrite-migrate-java",
		"RECIPE_VERSION=3.20.0",
		"RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17",
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected orw-cli to fail on unsupported attribution")
	}

	reportBytes, readErr := os.ReadFile(filepath.Join(outdir, contracts.ORWCLIReportFileName))
	if readErr != nil {
		t.Fatalf("read report.json: %v", readErr)
	}
	report, parseErr := contracts.ParseORWCLIReport(reportBytes)
	if parseErr != nil {
		t.Fatalf("parse report.json: %v\nreport=%s", parseErr, string(reportBytes))
	}
	if report.Success {
		t.Fatalf("report.success=true, expected false: %s", string(reportBytes))
	}
	if report.ErrorKind != contracts.ORWCLIErrorKindUnsupported {
		t.Fatalf("error_kind=%q, want %q", report.ErrorKind, contracts.ORWCLIErrorKindUnsupported)
	}
	if report.Reason != contracts.ORWCLIReasonTypeAttributionUnavailable {
		t.Fatalf("reason=%q, want %q", report.Reason, contracts.ORWCLIReasonTypeAttributionUnavailable)
	}
}

func TestOrwCLI_AutoExcludeGroovyParseFailures_RetriesOnceAndSucceeds(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	binDir := t.TempDir()
	callsFile := filepath.Join(t.TempDir(), "calls.txt")

	rewritePath := filepath.Join(binDir, "rewrite")
	rewriteScript := `#!/usr/bin/env bash
set -euo pipefail
calls_file="${REWRITE_CALLS_FILE:-}"
count=0
if [[ -n "$calls_file" && -f "$calls_file" ]]; then
  count="$(cat "$calls_file")"
fi
count=$((count + 1))
if [[ -n "$calls_file" ]]; then
  printf '%s' "$count" > "$calls_file"
fi
echo "[rewrite-stub] call=$count"
if [[ "$count" -eq 1 ]]; then
  echo "org.openrewrite.groovy.GroovyParsingException: Failed to parse build-client-swagger.gradle, cursor position likely inaccurate." >&2
  exit 1
fi
if [[ "${ORW_EXCLUDE_PATHS:-}" != *"**/*.proto"* ]]; then
  echo "[rewrite-stub] missing inherited exclude" >&2
  exit 61
fi
if [[ "${ORW_EXCLUDE_PATHS:-}" != *"**/build-client-swagger.gradle"* ]]; then
  echo "[rewrite-stub] missing auto exclude" >&2
  exit 62
fi
echo "[rewrite-stub] retry ok excludes=${ORW_EXCLUDE_PATHS:-}"
`
	if err := os.WriteFile(rewritePath, []byte(rewriteScript), 0o755); err != nil {
		t.Fatalf("write rewrite stub: %v", err)
	}

	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP=org.openrewrite.recipe",
		"RECIPE_ARTIFACT=rewrite-migrate-java",
		"RECIPE_VERSION=3.20.0",
		"RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17",
		"ORW_EXCLUDE_PATHS=**/*.proto",
		"ORW_AUTO_EXCLUDE_GROOVY_PARSE_FAILURES=true",
		"REWRITE_CALLS_FILE="+callsFile,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orw-cli failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	logBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLITransformLogName))
	if err != nil {
		t.Fatalf("read transform.log: %v", err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "Auto-exclude applied paths: **/build-client-swagger.gradle") {
		t.Fatalf("transform.log missing auto-exclude marker:\n%s", logText)
	}
	if !strings.Contains(logText, "[rewrite-stub] retry ok excludes=") {
		t.Fatalf("transform.log missing retry success marker:\n%s", logText)
	}

	callsRaw, err := os.ReadFile(callsFile)
	if err != nil {
		t.Fatalf("read calls file: %v", err)
	}
	if strings.TrimSpace(string(callsRaw)) != "2" {
		t.Fatalf("expected two rewrite invocations, got %q", strings.TrimSpace(string(callsRaw)))
	}

	reportBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLIReportFileName))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	report, err := contracts.ParseORWCLIReport(reportBytes)
	if err != nil {
		t.Fatalf("parse report.json: %v\nreport=%s", err, string(reportBytes))
	}
	if !report.Success {
		t.Fatalf("report.success=false, expected true: %s", string(reportBytes))
	}
}

func TestOrwCLI_AutoExcludeGroovyParseFailures_DisabledDoesNotRetry(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	binDir := t.TempDir()
	callsFile := filepath.Join(t.TempDir(), "calls.txt")

	rewritePath := filepath.Join(binDir, "rewrite")
	rewriteScript := `#!/usr/bin/env bash
set -euo pipefail
calls_file="${REWRITE_CALLS_FILE:-}"
count=0
if [[ -n "$calls_file" && -f "$calls_file" ]]; then
  count="$(cat "$calls_file")"
fi
count=$((count + 1))
if [[ -n "$calls_file" ]]; then
  printf '%s' "$count" > "$calls_file"
fi
echo "[rewrite-stub] call=$count"
echo "org.openrewrite.groovy.GroovyParsingException: Failed to parse build-client-swagger.gradle, cursor position likely inaccurate." >&2
exit 1
`
	if err := os.WriteFile(rewritePath, []byte(rewriteScript), 0o755); err != nil {
		t.Fatalf("write rewrite stub: %v", err)
	}

	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP=org.openrewrite.recipe",
		"RECIPE_ARTIFACT=rewrite-migrate-java",
		"RECIPE_VERSION=3.20.0",
		"RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17",
		"REWRITE_CALLS_FILE="+callsFile,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	if err := cmd.Run(); err == nil {
		t.Fatal("expected orw-cli to fail when auto-exclude is disabled")
	}

	callsRaw, err := os.ReadFile(callsFile)
	if err != nil {
		t.Fatalf("read calls file: %v", err)
	}
	if strings.TrimSpace(string(callsRaw)) != "1" {
		t.Fatalf("expected one rewrite invocation, got %q", strings.TrimSpace(string(callsRaw)))
	}

	logBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLITransformLogName))
	if err != nil {
		t.Fatalf("read transform.log: %v", err)
	}
	logText := string(logBytes)
	if strings.Contains(logText, "Auto-exclude applied paths:") {
		t.Fatalf("auto-exclude should not run when disabled:\n%s", logText)
	}

	reportBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLIReportFileName))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	report, err := contracts.ParseORWCLIReport(reportBytes)
	if err != nil {
		t.Fatalf("parse report.json: %v\nreport=%s", err, string(reportBytes))
	}
	if report.Success {
		t.Fatalf("report.success=true, expected false: %s", string(reportBytes))
	}
	if report.ErrorKind != contracts.ORWCLIErrorKindExecution {
		t.Fatalf("error_kind=%q, want %q", report.ErrorKind, contracts.ORWCLIErrorKindExecution)
	}
}

func TestOrwCLI_AutoExcludeGroovyParseFailures_MergesAndDedupesPaths(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	binDir := t.TempDir()
	callsFile := filepath.Join(t.TempDir(), "calls.txt")

	rewritePath := filepath.Join(binDir, "rewrite")
	rewriteScript := `#!/usr/bin/env bash
set -euo pipefail
calls_file="${REWRITE_CALLS_FILE:-}"
count=0
if [[ -n "$calls_file" && -f "$calls_file" ]]; then
  count="$(cat "$calls_file")"
fi
count=$((count + 1))
if [[ -n "$calls_file" ]]; then
  printf '%s' "$count" > "$calls_file"
fi
echo "[rewrite-stub] call=$count"
if [[ "$count" -eq 1 ]]; then
  echo "org.openrewrite.groovy.GroovyParsingException: Failed to parse build-client-swagger.gradle, cursor position likely inaccurate." >&2
  echo "org.openrewrite.groovy.GroovyParsingException: Failed to parse src/main/groovy/generated-client.gradle, cursor position likely inaccurate." >&2
  exit 1
fi
echo "[rewrite-stub] excludes=${ORW_EXCLUDE_PATHS:-}"
if [[ "${ORW_EXCLUDE_PATHS:-}" != *"**/*.proto"* ]]; then
  echo "[rewrite-stub] missing proto exclude" >&2
  exit 71
fi
if [[ "${ORW_EXCLUDE_PATHS:-}" != *"**/build-client-swagger.gradle"* ]]; then
  echo "[rewrite-stub] missing existing swagger exclude" >&2
  exit 72
fi
if [[ "${ORW_EXCLUDE_PATHS:-}" != *"src/main/groovy/generated-client.gradle"* ]]; then
  echo "[rewrite-stub] missing relative path exclude" >&2
  exit 73
fi
`
	if err := os.WriteFile(rewritePath, []byte(rewriteScript), 0o755); err != nil {
		t.Fatalf("write rewrite stub: %v", err)
	}

	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP=org.openrewrite.recipe",
		"RECIPE_ARTIFACT=rewrite-migrate-java",
		"RECIPE_VERSION=3.20.0",
		"RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17",
		"ORW_EXCLUDE_PATHS=**/*.proto,**/build-client-swagger.gradle",
		"ORW_AUTO_EXCLUDE_GROOVY_PARSE_FAILURES=true",
		"REWRITE_CALLS_FILE="+callsFile,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orw-cli failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	logBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLITransformLogName))
	if err != nil {
		t.Fatalf("read transform.log: %v", err)
	}
	logText := string(logBytes)
	if strings.Contains(logText, "Auto-exclude applied paths: **/build-client-swagger.gradle") {
		t.Fatalf("existing path must not be reported as newly added:\n%s", logText)
	}
	if !strings.Contains(logText, "Auto-exclude applied paths: src/main/groovy/generated-client.gradle") {
		t.Fatalf("expected new relative path to be reported:\n%s", logText)
	}
	if strings.Count(logText, "src/main/groovy/generated-client.gradle") < 2 {
		t.Fatalf("expected relative exclude in both reporting and retry invocation:\n%s", logText)
	}
}

func TestOrwCLI_PreflightAutoExcludesProto3AndEdition(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	binDir := t.TempDir()
	callsFile := filepath.Join(t.TempDir(), "calls.txt")

	proto3Path := filepath.Join(workspace, "src", "main", "resources", "google.type", "date.proto")
	editionPath := filepath.Join(workspace, "proto", "edition", "features.proto")
	proto2Path := filepath.Join(workspace, "proto", "legacy.proto")
	for _, path := range []string{proto3Path, editionPath, proto2Path} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
	}

	if err := os.WriteFile(proto3Path, []byte(`syntax = "proto3";
message Date { int32 year = 1; }
`), 0o644); err != nil {
		t.Fatalf("write proto3 file: %v", err)
	}
	if err := os.WriteFile(editionPath, []byte(`/* syntax = "proto2"; */
edition = "2023";
message Feature { string name = 1; }
`), 0o644); err != nil {
		t.Fatalf("write edition file: %v", err)
	}
	if err := os.WriteFile(proto2Path, []byte(`syntax = "proto2";
message Legacy { optional string name = 1; }
`), 0o644); err != nil {
		t.Fatalf("write proto2 file: %v", err)
	}

	rewritePath := filepath.Join(binDir, "rewrite")
	rewriteScript := `#!/usr/bin/env bash
set -euo pipefail
calls_file="${REWRITE_CALLS_FILE:-}"
count=0
if [[ -n "$calls_file" && -f "$calls_file" ]]; then
  count="$(cat "$calls_file")"
fi
count=$((count + 1))
if [[ -n "$calls_file" ]]; then
  printf '%s' "$count" > "$calls_file"
fi
echo "[rewrite-stub] call=$count"
echo "[rewrite-stub] excludes=${ORW_EXCLUDE_PATHS:-}"
if [[ "${ORW_EXCLUDE_PATHS:-}" != *"src/main/resources/google.type/date.proto"* ]]; then
  echo "[rewrite-stub] missing proto3 preflight exclude" >&2
  exit 81
fi
if [[ "${ORW_EXCLUDE_PATHS:-}" != *"proto/edition/features.proto"* ]]; then
  echo "[rewrite-stub] missing edition preflight exclude" >&2
  exit 82
fi
if [[ "${ORW_EXCLUDE_PATHS:-}" == *"proto/legacy.proto"* ]]; then
  echo "[rewrite-stub] proto2 file must not be auto-excluded" >&2
  exit 83
fi
`
	if err := os.WriteFile(rewritePath, []byte(rewriteScript), 0o755); err != nil {
		t.Fatalf("write rewrite stub: %v", err)
	}

	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP=org.openrewrite.recipe",
		"RECIPE_ARTIFACT=rewrite-migrate-java",
		"RECIPE_VERSION=3.20.0",
		"RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17",
		"REWRITE_CALLS_FILE="+callsFile,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orw-cli failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	callsRaw, err := os.ReadFile(callsFile)
	if err != nil {
		t.Fatalf("read calls file: %v", err)
	}
	if strings.TrimSpace(string(callsRaw)) != "1" {
		t.Fatalf("expected one rewrite invocation, got %q", strings.TrimSpace(string(callsRaw)))
	}

	logBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLITransformLogName))
	if err != nil {
		t.Fatalf("read transform.log: %v", err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "Proto pre-scan auto-exclude candidates:") {
		t.Fatalf("transform.log missing proto pre-scan marker:\n%s", logText)
	}
	if !strings.Contains(logText, "src/main/resources/google.type/date.proto") {
		t.Fatalf("transform.log missing proto3 path:\n%s", logText)
	}
	if !strings.Contains(logText, "proto/edition/features.proto") {
		t.Fatalf("transform.log missing edition path:\n%s", logText)
	}
	if strings.Contains(logText, "proto/legacy.proto") {
		t.Fatalf("transform.log should not include proto2 path:\n%s", logText)
	}

	reportBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLIReportFileName))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	report, err := contracts.ParseORWCLIReport(reportBytes)
	if err != nil {
		t.Fatalf("parse report.json: %v\nreport=%s", err, string(reportBytes))
	}
	if !report.Success {
		t.Fatalf("report.success=false, expected true: %s", string(reportBytes))
	}
}

func TestOrwCLI_SelfTestWritesSuccessReport(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	migScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", migScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(), "MIGS_SELF_TEST=1")

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("orw-cli self-test failed: %v\n%s", err, string(out))
	}

	reportBytes, err := os.ReadFile(filepath.Join(outdir, contracts.ORWCLIReportFileName))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	report, err := contracts.ParseORWCLIReport(reportBytes)
	if err != nil {
		t.Fatalf("parse report.json: %v\nreport=%s", err, string(reportBytes))
	}
	if !report.Success {
		t.Fatalf("self-test report.success=false: %s", string(reportBytes))
	}
}

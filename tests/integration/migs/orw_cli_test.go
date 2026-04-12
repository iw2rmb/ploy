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
		filepath.Join(root, "images", "orw", "orw-cli-maven", "orw-cli.sh"),
		filepath.Join(root, "images", "orw", "orw-cli-gradle", "orw-cli.sh"),
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
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) workspace="${2:-}"; shift 2 ;;
    *) shift ;;
  esac
done
echo "[rewrite-stub] apply invoked"
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
		"RECIPE_VERSION=3.20.0",
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
	if strings.Contains(logText, "[mvn-stub] should not run") || strings.Contains(logText, "[gradle-stub] should not run") {
		t.Fatalf("transform.log shows build-tool invocation:\n%s", logText)
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

func TestOrwCLI_WarnsOnLegacyExcludeAliasesAndExportsCanonicalExcludeEnv(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outdir := t.TempDir()
	binDir := t.TempDir()

	rewritePath := filepath.Join(binDir, "rewrite")
	rewriteScript := `#!/usr/bin/env bash
set -euo pipefail
if [[ "${ORW_EXCLUDE_PATHS+x}" == "x" ]]; then
  echo "[rewrite-stub] ORW_EXCLUDE_PATHS_SET"
else
  echo "[rewrite-stub] ORW_EXCLUDE_PATHS_UNSET"
fi
echo "[rewrite-stub] ORW_EXCLUDES=${ORW_EXCLUDES:-}"
echo "[rewrite-stub] ORW_INCLUDES=${ORW_INCLUDES:-}"
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
		"ORW_EXCLUDES=**/*.proto",
		"ORW_INCLUDES=src/main/java/**",
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
	if !strings.Contains(logText, "Warning: ORW_EXCLUDES/ORW_INCLUDES are unsupported; use ORW_EXCLUDE_PATHS.") {
		t.Fatalf("transform.log missing legacy-alias warning:\n%s", logText)
	}
	if !strings.Contains(logText, "[rewrite-stub] ORW_EXCLUDE_PATHS_SET") {
		t.Fatalf("transform.log missing canonical ORW_EXCLUDE_PATHS marker:\n%s", logText)
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

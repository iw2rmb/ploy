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
		filepath.Join(root, "deploy", "images", "mig", "orw-cli", "orw-cli.sh"),
		filepath.Join(root, "deploy", "images", "mig", "orw-cli-maven", "orw-cli.sh"),
		filepath.Join(root, "deploy", "images", "mig", "orw-cli-gradle", "orw-cli.sh"),
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

	modScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
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

	modScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
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
	modScript := resolveORWCLIScript(t)
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(), "MODS_SELF_TEST=1")

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

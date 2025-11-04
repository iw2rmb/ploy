package integration

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Extracts values from tests/e2e/mods/scenario-orw-pass.sh
func parseScenarioORWPass(content string) (repoURL, baseRef, targetRef, group, artifact, version, classname, plugin string) {
	// defaults align with the scenario script
	repoURL = "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git"
	baseRef = "main"
	targetRef = "e2e/success"
	group = "org.openrewrite.recipe"
	artifact = "rewrite-java-17"
	version = "2.6.0"
	classname = "org.openrewrite.java.migrate.UpgradeToJava17"
	plugin = "6.18.0"

	// REPO=${PLOY_E2E_REPO_OVERRIDE:-<url>}
	if m := regexp.MustCompile(`(?m)^REPO=\$\{[^:]+:-([^}]+)\}`).FindStringSubmatch(content); len(m) == 2 {
		repoURL = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?m)^TARGET_REF=([^\n]+)$`).FindStringSubmatch(content); len(m) == 2 {
		targetRef = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?m)^RECIPE_GROUP=([^\n]+)$`).FindStringSubmatch(content); len(m) == 2 {
		group = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?m)^RECIPE_ARTIFACT=([^\n]+)$`).FindStringSubmatch(content); len(m) == 2 {
		artifact = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?m)^RECIPE_VERSION=([^\n]+)$`).FindStringSubmatch(content); len(m) == 2 {
		version = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?m)^RECIPE_CLASSNAME=([^\n]+)$`).FindStringSubmatch(content); len(m) == 2 {
		classname = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?m)^MAVEN_PLUGIN_VERSION=([^\n]+)$`).FindStringSubmatch(content); len(m) == 2 {
		plugin = strings.TrimSpace(m[1])
	}
	_ = repoURL
	_ = baseRef
	_ = targetRef // kept for future assertions if desired
	return
}

// writes a simple mvn shim that appends a comment to pom.xml reflecting the recipe
// no mvn shim: we require real mvn available for this test

func run(t *testing.T, name string, args ...string) (string, string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("run %s %v: %v\nstdout=%s\nstderr=%s", name, args, err, out.String(), errb.String())
	}
	return out.String(), errb.String()
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, _ := run(t, "git", "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out)
}

// Test that running mods/mod-orw/mod-orw.sh with real recipe coordinates from
// the scenario script produces a git diff in the workspace. We stub mvn so the
// test is hermetic and fast, but we preserve the real recipe args.
func TestModORW_ProducesDiff_WithScenarioData(t *testing.T) {
	// Read scenario for real coordinates and defaults.
	scenarioPath := filepath.Join(repoRoot(t), "tests", "e2e", "mods", "scenario-orw-pass.sh")
	scenarioBytes, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatalf("read scenario script: %v", err)
	}
	repoURL, _, _, group, artifact, version, classname, plugin := parseScenarioORWPass(string(scenarioBytes))

	// Skip if mvn not available (keeps default dev runs fast)
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("mvn not found in PATH; skipping real mod-orw integration test")
	}

	// Prepare workspace by shallow-cloning the real repo base ref (main)
	workspace := t.TempDir()
	baseRef := "main"
	// Clone quietly to reduce noise
	cmdClone := exec.Command("git", "clone", "--depth", "1", "--branch", baseRef, repoURL, workspace)
	if out, err := cmdClone.CombinedOutput(); err != nil {
		t.Fatalf("git clone failed: %v\n%s", err, string(out))
	}

	// Prepare out dir for mod outputs
	outdir := t.TempDir()

	// Run the mod-orw script with real coordinates from scenario
	modScript := filepath.Join(repoRoot(t), "mods", "mod-orw", "mod-orw.sh")
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP="+group,
		"RECIPE_ARTIFACT="+artifact,
		"RECIPE_VERSION="+version,
		"RECIPE_CLASSNAME="+classname,
		"MAVEN_PLUGIN_VERSION="+plugin,
	)
	// stream to aid debugging on failure
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mod-orw: %v", err)
	}
	go func() {
		io := bufio.NewScanner(stdout)
		for io.Scan() {
		}
	}()
	go func() {
		io := bufio.NewScanner(stderr)
		for io.Scan() {
		}
	}()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("mod-orw failed: %v", err)
	}

	// Assert a git diff exists (ORW applies changes under the working tree)
	diffOut, _ := run(t, "git", "-C", workspace, "diff", "--patch")
	if strings.TrimSpace(diffOut) == "" {
		t.Fatalf("expected non-empty diff, got empty")
	}

	// Also verify the report.json exists in out dir and indicates success
	reportPath := filepath.Join(outdir, "report.json")
	b, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	if !strings.Contains(string(b), "\"success\": true") {
		t.Fatalf("report.json does not indicate success: %s", string(b))
	}
}

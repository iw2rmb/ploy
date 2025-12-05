package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestOrwMaven_MavenWorkspace_AppliesRecipe verifies that the orw-maven script
// applies OpenRewrite recipes to Maven projects. Uses real mvn if available,
// otherwise skips.
func TestOrwMaven_MavenWorkspace_AppliesRecipe(t *testing.T) {
	// Skip if mvn not available (keeps default dev runs fast).
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("mvn not found in PATH; skipping orw-maven integration test")
	}

	// Use the scenario script defaults for recipe coordinates.
	scenarioPath := filepath.Join(repoRoot(t), "tests", "e2e", "mods", "scenario-orw-pass.sh")
	scenarioBytes, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatalf("read scenario script: %v", err)
	}
	repoURL, _, _, group, artifact, version, classname, plugin := parseScenarioORWPass(string(scenarioBytes))

	// Prepare workspace by shallow-cloning the real repo base ref (main).
	workspace := t.TempDir()
	baseRef := "main"
	cmdClone := exec.Command("git", "clone", "--depth", "1", "--branch", baseRef, repoURL, workspace)
	if out, err := cmdClone.CombinedOutput(); err != nil {
		t.Fatalf("git clone failed: %v\n%s", err, string(out))
	}

	outdir := t.TempDir()

	// Run the orw-maven script with recipe coordinates.
	modScript := filepath.Join(repoRoot(t), "docker", "mods", "orw-maven", "orw-maven.sh")
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP="+group,
		"RECIPE_ARTIFACT="+artifact,
		"RECIPE_VERSION="+version,
		"RECIPE_CLASSNAME="+classname,
		"MAVEN_PLUGIN_VERSION="+plugin,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orw-maven failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	// Assert a git diff exists (ORW applies changes under the working tree).
	diffOut, _ := run(t, "git", "-C", workspace, "diff", "--patch")
	if strings.TrimSpace(diffOut) == "" {
		t.Fatalf("expected non-empty diff, got empty")
	}

	// Verify the report.json exists and indicates success.
	reportPath := filepath.Join(outdir, "report.json")
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	if !strings.Contains(string(reportBytes), "\"success\": true") {
		t.Fatalf("report.json does not indicate success: %s", string(reportBytes))
	}
}

// TestOrwMaven_NonMavenWorkspace_Fails verifies that orw-maven fails with exit 5
// when the workspace does not contain pom.xml.
func TestOrwMaven_NonMavenWorkspace_Fails(t *testing.T) {
	workspace := t.TempDir()
	outdir := t.TempDir()

	// Create a Gradle build file instead of pom.xml.
	buildGradlePath := filepath.Join(workspace, "build.gradle.kts")
	if err := os.WriteFile(buildGradlePath, []byte("// Gradle project\n"), 0o644); err != nil {
		t.Fatalf("write build.gradle.kts: %v", err)
	}

	modScript := filepath.Join(repoRoot(t), "docker", "mods", "orw-maven", "orw-maven.sh")
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP=org.openrewrite.recipe",
		"RECIPE_ARTIFACT=rewrite-java-17",
		"RECIPE_VERSION=2.6.0",
		"RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected orw-maven to fail on non-Maven workspace")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	// Exit code 5 indicates missing build file.
	if exitErr.ExitCode() != 5 {
		t.Fatalf("expected exit code 5, got %d\nstderr: %s", exitErr.ExitCode(), stderr.String())
	}
}

// TestOrwMaven_SelfTest verifies the self-test mode writes a success report.
func TestOrwMaven_SelfTest(t *testing.T) {
	workspace := t.TempDir()
	outdir := t.TempDir()

	modScript := filepath.Join(repoRoot(t), "docker", "mods", "orw-maven", "orw-maven.sh")
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(), "MODS_SELF_TEST=1")

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("orw-maven self-test failed: %v\n%s", err, string(out))
	}

	reportPath := filepath.Join(outdir, "report.json")
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	if !strings.Contains(string(reportBytes), "\"self_test\":true") {
		t.Fatalf("report.json does not indicate self_test: %s", string(reportBytes))
	}
}

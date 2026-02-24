package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestOrwGradle_GradleWorkspace_UsesWrapper verifies that the orw-gradle script
// invokes ./gradlew when present. Uses a stubbed gradlew for hermetic testing.
func TestOrwGradle_GradleWorkspace_UsesWrapper(t *testing.T) {
	workspace := t.TempDir()

	// Create a minimal Kotlin DSL Gradle build file.
	buildGradlePath := filepath.Join(workspace, "build.gradle.kts")
	if err := os.WriteFile(buildGradlePath, []byte(`// dummy Kotlin DSL gradle build for tests
plugins {
}
`), 0o644); err != nil {
		t.Fatalf("write build.gradle.kts: %v", err)
	}

	// Stub ./gradlew so the script can invoke it without requiring real Gradle.
	gradlewPath := filepath.Join(workspace, "gradlew")
	gradlewScript := `#!/usr/bin/env bash
echo "[gradle-stub] rewriteRun invoked with args: $@"
`
	if err := os.WriteFile(gradlewPath, []byte(gradlewScript), 0o755); err != nil {
		t.Fatalf("write gradlew stub: %v", err)
	}

	outdir := t.TempDir()

	// Use scenario script defaults for recipe coordinates.
	scenarioPath := filepath.Join(repoRoot(t), "tests", "e2e", "migs", "scenario-orw-pass.sh")
	scenarioBytes, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatalf("read scenario script: %v", err)
	}
	_, _, _, group, artifact, version, classname, _ := parseScenarioORWPass(string(scenarioBytes))

	modScript := filepath.Join(repoRoot(t), "deploy", "images", "migs", "orw-gradle", "orw-gradle.sh")
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
	// Remove system gradle from PATH so the script uses ./gradlew.
	filteredPath := filterPath("gradle")
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP="+group,
		"RECIPE_ARTIFACT="+artifact,
		"RECIPE_VERSION="+version,
		"RECIPE_CLASSNAME="+classname,
		"PATH="+filteredPath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orw-gradle (wrapper) failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	// Verify that transform.log contains the gradle stub marker.
	logPath := filepath.Join(outdir, "transform.log")
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read transform.log: %v", err)
	}
	if !strings.Contains(string(logBytes), "[gradle-stub] rewriteRun invoked") {
		t.Fatalf("transform.log does not contain gradle stub marker:\n%s", string(logBytes))
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

// TestOrwGradle_UsesExistingRewriteYAML verifies that when rewrite.yml is present
// in the workspace, orw-gradle uses its top-level recipe name as the active recipe.
func TestOrwGradle_UsesExistingRewriteYAML(t *testing.T) {
	workspace := t.TempDir()

	// Minimal Kotlin DSL Gradle build file.
	buildGradlePath := filepath.Join(workspace, "build.gradle.kts")
	if err := os.WriteFile(buildGradlePath, []byte(`plugins {
}
`), 0o644); err != nil {
		t.Fatalf("write build.gradle.kts: %v", err)
	}

	// Stub ./gradlew so the script can invoke it without requiring real Gradle.
	gradlewPath := filepath.Join(workspace, "gradlew")
	gradlewScript := `#!/usr/bin/env bash
echo "[gradle-stub] rewriteRun invoked with args: $@"
`
	if err := os.WriteFile(gradlewPath, []byte(gradlewScript), 0o755); err != nil {
		t.Fatalf("write gradlew stub: %v", err)
	}

	// Provide rewrite.yml with a named recipe.
	rewritePath := filepath.Join(workspace, "rewrite.yml")
	rewriteContent := []byte(`type: specs.openrewrite.org/v1beta/recipe
name: PloyYamlRecipe
recipeList:
  - org.openrewrite.java.migrate.UpgradeToJava17
`)
	if err := os.WriteFile(rewritePath, rewriteContent, 0o644); err != nil {
		t.Fatalf("write rewrite.yml: %v", err)
	}

	outdir := t.TempDir()

	// Use scenario script defaults for recipe coordinates.
	scenarioPath := filepath.Join(repoRoot(t), "tests", "e2e", "migs", "scenario-orw-pass.sh")
	scenarioBytes, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatalf("read scenario script: %v", err)
	}
	_, _, _, group, artifact, version, classname, _ := parseScenarioORWPass(string(scenarioBytes))

	modScript := filepath.Join(repoRoot(t), "deploy", "images", "migs", "orw-gradle", "orw-gradle.sh")
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
	// Remove system gradle from PATH so the script uses ./gradlew.
	filteredPath := filterPath("gradle")
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP="+group,
		"RECIPE_ARTIFACT="+artifact,
		"RECIPE_VERSION="+version,
		"RECIPE_CLASSNAME="+classname,
		"PATH="+filteredPath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orw-gradle (rewrite.yml) failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	// Verify that transform.log shows the YAML recipe name as active recipe.
	logPath := filepath.Join(outdir, "transform.log")
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read transform.log: %v", err)
	}
	if !strings.Contains(string(logBytes), "-Drewrite.activeRecipes=PloyYamlRecipe") {
		t.Fatalf("transform.log does not show YAML recipe as active:\n%s", string(logBytes))
	}
}

// TestOrwGradle_NonGradleWorkspace_Fails verifies that orw-gradle fails with exit 5
// when the workspace does not contain build.gradle or build.gradle.kts.
func TestOrwGradle_NonGradleWorkspace_Fails(t *testing.T) {
	workspace := t.TempDir()
	outdir := t.TempDir()

	// Create a Maven pom.xml instead of Gradle build file.
	pomPath := filepath.Join(workspace, "pom.xml")
	if err := os.WriteFile(pomPath, []byte("<project></project>\n"), 0o644); err != nil {
		t.Fatalf("write pom.xml: %v", err)
	}

	modScript := filepath.Join(repoRoot(t), "deploy", "images", "migs", "orw-gradle", "orw-gradle.sh")
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
		t.Fatal("expected orw-gradle to fail on non-Gradle workspace")
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

// TestOrwGradle_GroovyDSL_Fails verifies that orw-gradle fails with exit 5
// when the workspace contains build.gradle (Groovy DSL) instead of Kotlin DSL.
func TestOrwGradle_GroovyDSL_Fails(t *testing.T) {
	workspace := t.TempDir()
	outdir := t.TempDir()

	// Create a Groovy DSL build file.
	buildGradlePath := filepath.Join(workspace, "build.gradle")
	if err := os.WriteFile(buildGradlePath, []byte("// Groovy DSL\n"), 0o644); err != nil {
		t.Fatalf("write build.gradle: %v", err)
	}

	// Stub ./gradlew so the script finds a Gradle command before checking DSL style.
	gradlewPath := filepath.Join(workspace, "gradlew")
	gradlewScript := `#!/usr/bin/env bash
echo "[gradle-stub] invoked"
`
	if err := os.WriteFile(gradlewPath, []byte(gradlewScript), 0o755); err != nil {
		t.Fatalf("write gradlew stub: %v", err)
	}

	modScript := filepath.Join(repoRoot(t), "deploy", "images", "migs", "orw-gradle", "orw-gradle.sh")
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
	// Remove system gradle from PATH so the script uses ./gradlew.
	filteredPath := filterPath("gradle")
	cmd.Env = append(os.Environ(),
		"RECIPE_GROUP=org.openrewrite.recipe",
		"RECIPE_ARTIFACT=rewrite-java-17",
		"RECIPE_VERSION=2.6.0",
		"RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17",
		"PATH="+filteredPath,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected orw-gradle to fail on Groovy DSL workspace")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	// Exit code 5 indicates unsupported build style (Groovy).
	if exitErr.ExitCode() != 5 {
		t.Fatalf("expected exit code 5, got %d\nstderr: %s", exitErr.ExitCode(), stderr.String())
	}
}

// TestOrwGradle_SelfTest verifies the self-test mode writes a success report.
func TestOrwGradle_SelfTest(t *testing.T) {
	workspace := t.TempDir()
	outdir := t.TempDir()

	modScript := filepath.Join(repoRoot(t), "deploy", "images", "migs", "orw-gradle", "orw-gradle.sh")
	cmd := exec.Command("bash", modScript, "--apply", "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(), "MODS_SELF_TEST=1")

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("orw-gradle self-test failed: %v\n%s", err, string(out))
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

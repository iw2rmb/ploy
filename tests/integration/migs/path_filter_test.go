package integration

import (
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// filterPath returns PATH with any entry containing the given substring removed.
// This helps isolate tests from system-installed binaries.
func filterPath(exclude string) string {
	original := os.Getenv("PATH")
	parts := strings.Split(original, string(os.PathListSeparator))
	var filtered []string
	for _, p := range parts {
		if !strings.Contains(p, exclude) {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, string(os.PathListSeparator))
}

// run executes a command and returns stdout/stderr, failing the test on error.
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

// repoRoot returns the top-level directory of the Git repository.
func repoRoot(t *testing.T) string {
	t.Helper()
	out, _ := run(t, "git", "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out)
}

// parseScenarioORWPass extracts values from tests/e2e/migs/scenario-orw-pass.sh.
// Defaults align with the scenario script; parsing overrides them if found.
func parseScenarioORWPass(content string) (repoURL, baseRef, targetRef, group, artifact, version, classname string) {
	repoURL = "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git"
	baseRef = "main"
	targetRef = "e2e/success"
	group = "org.openrewrite.recipe"
	artifact = "rewrite-migrate-java"
	version = "3.20.0"
	classname = "org.openrewrite.java.migrate.UpgradeToJava17"

	// Parse overrides from the scenario script content.
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
	_ = repoURL
	_ = baseRef
	_ = targetRef // kept for future assertions if desired
	return
}

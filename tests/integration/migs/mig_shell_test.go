package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestModShell_ExecutesScriptAndWritesReport verifies that mig-shell executes
// the requested script in the workspace and writes a run report.
func TestModShell_ExecutesScriptAndWritesReport(t *testing.T) {
	workspace := t.TempDir()
	outdir := t.TempDir()

	// Script that writes a marker file into the workspace.
	scriptPath := filepath.Join(workspace, "mig-shell-script.sh")
	script := `#!/usr/bin/env bash
set -euo pipefail
echo "from-mig-shell" > rewrite.yml
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	modScript := filepath.Join(repoRoot(t), "deploy", "images", "shell", "mig-shell.sh")
	cmd := exec.Command("bash", modScript, "--dir", workspace, "--out", outdir)
	cmd.Env = append(os.Environ(),
		"MOD_SHELL_SCRIPT="+scriptPath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mig-shell failed: %v\nstdout/stderr:\n%s", err, string(out))
	}

	// Verify the script ran and produced rewrite.yml.
	rewritePath := filepath.Join(workspace, "rewrite.yml")
	content, err := os.ReadFile(rewritePath)
	if err != nil {
		t.Fatalf("read rewrite.yml: %v", err)
	}
	if !strings.Contains(string(content), "from-mig-shell") {
		t.Fatalf("rewrite.yml does not contain expected content: %s", string(content))
	}

	// Verify that shell-run.json report exists and indicates success.
	reportPath := filepath.Join(outdir, "shell-run.json")
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read shell-run.json: %v", err)
	}
	if !strings.Contains(string(reportBytes), "\"success\":true") {
		t.Fatalf("shell-run.json does not indicate success: %s", string(reportBytes))
	}
}

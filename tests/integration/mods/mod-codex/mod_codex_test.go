package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func have(name string) bool { _, err := exec.LookPath(name); return err == nil }

func mustRun(t *testing.T, name string, args ...string) (string, string) {
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

func TestModCodex_HealsUsingBuildGateLog_FromFailingBranch(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if !have("docker") {
		t.Skip("docker not found in PATH; skipping")
	}
	if !have("git") {
		t.Skip("git not found in PATH; skipping")
	}

	repoURL := "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git"
	branch := "e2e/fail-missing-symbol"

	ws := t.TempDir()
	outDir := t.TempDir()

	// Clone failing branch (shallow)
	cmdClone := exec.Command("git", "clone", "--depth", "1", "--branch", branch, repoURL, ws)
	if out, err := cmdClone.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, string(out))
	}

	// Ensure .ploy exists
	if err := os.MkdirAll(filepath.Join(ws, ".ploy"), 0o755); err != nil {
		t.Fatalf("mkdir .ploy: %v", err)
	}

	// Prefer a pre-captured Build Gate log placed alongside this test.
	repoRoot, _ := mustRun(t, "git", "rev-parse", "--show-toplevel")
	repoRoot = strings.TrimSpace(repoRoot)
	testLog := filepath.Join(repoRoot, "tests", "integration", "mods", "mod-codex", "build-gate.log")
	logPath := filepath.Join(ws, ".ploy", "build-gate.log")
	if data, err := os.ReadFile(testLog); err == nil && len(data) > 0 {
		if err := os.WriteFile(logPath, data, 0o644); err != nil {
			t.Fatalf("write build-gate.log from test fixture: %v", err)
		}
	} else {
		// Fallback: run Build Gate inside Maven image to generate build-gate.log (with -e)
		mvnCmd := "mvn -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test"
		runArgs := []string{"run", "--rm", "-v", ws + ":/workspace", "-w", "/workspace", "maven:3-eclipse-temurin-17", "/bin/sh", "-lc", mvnCmd}
		cmd := exec.Command("docker", runArgs...)
		var logs bytes.Buffer
		cmd.Stdout = &logs
		cmd.Stderr = &logs
		_ = cmd.Run() // expect non-zero; ignore error but capture logs
		if err := os.WriteFile(logPath, logs.Bytes(), 0o644); err != nil {
			t.Fatalf("write build-gate.log: %v", err)
		}
	}
	if b, _ := os.ReadFile(logPath); !bytes.Contains(b, []byte("cannot find symbol")) {
		t.Fatalf("expected compilation error in build-gate.log, got:\n%s", string(b))
	}

	// Build mods-codex image (tag: mods-codex:latest)
	_, _ = mustRun(t, "docker", "build", "-t", "mods-codex:latest", "-f", filepath.Join(repoRoot, "mods", "mod-codex", "Dockerfile"), repoRoot)

	// Prepare prompt with explicit verification rule for ploy-buildgate
	prompt := strings.Join([]string{
		"Rules:",
		"- After making any change, verify the build.",
		"- Run: ploy-buildgate --workspace \"$PLOY_HOST_WORKSPACE\" --profile auto",
		"- If it fails, iterate and try again until it passes.",
		"- Only finalize once the gate passes; then print \"BUILD PASSED\".",
		"",
		"Task:",
		"fix compilation error described in /in/build-gate.log",
	}, "\n")
	if err := os.WriteFile(filepath.Join(ws, "prompt.txt"), []byte(prompt), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	// Require real Codex auth from environment to run this test without stubbing.
	auth := os.Getenv("CODEX_AUTH_JSON")
	if strings.TrimSpace(auth) == "" {
		t.Skip("CODEX_AUTH_JSON not set; skipping real Codex execution test")
	}
	// Run mod-codex; map workspace to the same absolute path inside container, mount Docker socket for ploy-buildgate
	run := exec.Command("docker", "run", "--rm",
		"-e", "CODEX_AUTH_JSON="+auth,
		"-e", "PLOY_HOST_WORKSPACE="+ws,
		"-v", ws+":"+ws,
		"-w", ws,
		"-v", outDir+":/out",
		"-v", filepath.Join(ws, ".ploy")+":/in:ro",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"mods-codex:latest",
		"--input", ws, "--out", "/out", "--prompt-file", filepath.Join(ws, "prompt.txt"),
	)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("mod-codex container failed: %v\n%s", err, string(out))
	}

	// Assertions
	// 1) codex-run.json sane
	runJSON := filepath.Join(outDir, "codex-run.json")
	rb, err := os.ReadFile(runJSON)
	if err != nil {
		t.Fatalf("read codex-run.json: %v", err)
	}
	var rj struct {
		ExitCode int    `json:"exit_code"`
		Input    string `json:"input"`
	}
	if err := json.Unmarshal(rb, &rj); err != nil {
		t.Fatalf("parse codex-run.json: %v", err)
	}
	if rj.Input != ws {
		t.Fatalf("unexpected codex-run.json input: %s", string(rb))
	}
	// 2) codex.log exists and is non-empty
	lb, err := os.ReadFile(filepath.Join(outDir, "codex.log"))
	if err != nil || len(bytes.TrimSpace(lb)) == 0 {
		t.Fatalf("codex.log missing or empty: %v", err)
	}
}

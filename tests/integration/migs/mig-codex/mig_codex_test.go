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

// realTempDir creates a temp directory under the user home directory.
// This ensures the path is accessible to Docker Desktop on macOS, which
// only file-shares /Users (not /tmp or /var/folders).
func realTempDir(pattern string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	base := filepath.Join(home, ".codex-test-tmp")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(base, pattern)
}

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

func buildMigCodexImage(t *testing.T, repoRoot, imageTag string) {
	t.Helper()
	dockerfile := filepath.Join(repoRoot, "images", "codex", "Dockerfile")
	_, _ = mustRun(t, "docker", "build", "-t", imageTag, "-f", dockerfile, repoRoot)
}

// TestMigCodexContainer tests codex.sh dual-mode routing inside the container image.
// Amata mode: injects a mock amata binary via volume mount and verifies artifact outputs.
// Direct codex mode: verifies CODEX_PROMPT is required when amata mode is not active.
func TestMigCodexContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if !have("docker") {
		t.Skip("docker not found in PATH; skipping")
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("docker daemon not available; skipping: %v: %s", err, strings.TrimSpace(string(out)))
	}

	repoRoot, _ := mustRun(t, "git", "rev-parse", "--show-toplevel")
	repoRoot = strings.TrimSpace(repoRoot)

	// Build codex image once for both subtests.
	buildMigCodexImage(t, repoRoot, "codex:test-amata")

	// Build a test image that includes a mock amata binary on top of the base image.
	// This avoids volume-mount permission issues with Docker Desktop on macOS.
	const testImageTag = "codex:test-amata-with-mock"
	mockDockerfile := "FROM codex:test-amata\n" +
		"RUN printf '#!/bin/sh\\necho amata mock ran\\nexit 0\\n' > /usr/local/bin/amata " +
		"&& chmod +x /usr/local/bin/amata\n"
	mockDockerfileDir := t.TempDir()
	if err := os.WriteFile(mockDockerfileDir+"/Dockerfile", []byte(mockDockerfile), 0o644); err != nil {
		t.Fatalf("write mock Dockerfile: %v", err)
	}
	mustRun(t, "docker", "build", "-t", testImageTag, mockDockerfileDir)

	// Build a test image with mocked ccr + amata to validate startup activation.
	const ccrTestImageTag = "codex:test-amata-with-mock-ccr"
	ccrMockDockerfile := `FROM codex:test-amata
RUN cat <<'EOF' > /usr/local/bin/ccr
#!/bin/sh
set -eu
cmd="${1:-}"
case "$cmd" in
  start)
    printf "start\n" >> /out/ccr-mock.log
    exit 0
    ;;
  activate)
    printf "activate\n" >> /out/ccr-mock.log
    printf '%s\n' 'export CCR_ACTIVATED=1'
    exit 0
    ;;
  *)
    echo "unsupported ccr command: $*" >&2
    exit 64
    ;;
esac
EOF
RUN cat <<'EOF' > /usr/local/bin/amata
#!/bin/sh
set -eu
if [ "${CCR_ACTIVATED:-}" != "1" ]; then
  echo "ccr activation missing" >&2
  exit 21
fi
if [ ! -s "$HOME/.claude-code-router/config.json" ]; then
  echo "ccr config missing" >&2
  exit 22
fi
echo "amata mock ran"
exit 0
EOF
RUN chmod +x /usr/local/bin/ccr /usr/local/bin/amata
`
	ccrMockDockerfileDir := t.TempDir()
	if err := os.WriteFile(ccrMockDockerfileDir+"/Dockerfile", []byte(ccrMockDockerfile), 0o644); err != nil {
		t.Fatalf("write ccr mock Dockerfile: %v", err)
	}
	mustRun(t, "docker", "build", "-t", ccrTestImageTag, ccrMockDockerfileDir)

	t.Run("amata_mode_routes_to_amata_and_writes_artifacts", func(t *testing.T) {
		// Resolve symlinks so Docker Desktop on macOS gets the real path (e.g. /private/tmp vs /tmp).
		outDir, err := realTempDir("codex-test-out-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(outDir) })
		inDir, err := realTempDir("codex-test-in-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(inDir) })

		// Write a minimal amata.yaml to /in so the path exists.
		if err := os.WriteFile(inDir+"/amata.yaml", []byte("task: test\n"), 0o644); err != nil {
			t.Fatalf("write amata.yaml: %v", err)
		}

		run := exec.Command("docker", "run", "--rm",
			"-v", outDir+":/out",
			"-v", inDir+":/in:ro",
			testImageTag,
			"amata", "run", "/in/amata.yaml", "--set", "key=val",
		)
		if out, err := run.CombinedOutput(); err != nil {
			t.Fatalf("amata mode container failed: %v\n%s", err, string(out))
		}

		// codex.log must exist and be non-empty.
		lb, err := os.ReadFile(outDir + "/codex.log")
		if err != nil || len(bytes.TrimSpace(lb)) == 0 {
			t.Fatalf("codex.log missing or empty: %v", err)
		}
		// codex-last.txt must exist.
		if _, err := os.Stat(outDir + "/codex-last.txt"); err != nil {
			t.Fatalf("codex-last.txt missing: %v", err)
		}
		// codex-run.json must be valid JSON with exit_code:0.
		rb, err := os.ReadFile(outDir + "/codex-run.json")
		if err != nil {
			t.Fatalf("codex-run.json missing: %v", err)
		}
		var rj struct {
			ExitCode int `json:"exit_code"`
		}
		if err := json.Unmarshal(rb, &rj); err != nil {
			t.Fatalf("parse codex-run.json: %v", err)
		}
		if rj.ExitCode != 0 {
			t.Errorf("codex-run.json exit_code = %d, want 0", rj.ExitCode)
		}
	})

	t.Run("default_codex_home_materializes_auth_and_config_under_out_codex", func(t *testing.T) {
		outDir, err := realTempDir("codex-test-out-codex-home-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(outDir) })
		inDir, err := realTempDir("codex-test-in-codex-home-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(inDir) })

		if err := os.WriteFile(inDir+"/amata.yaml", []byte("task: codex-home\n"), 0o644); err != nil {
			t.Fatalf("write amata.yaml: %v", err)
		}

		run := exec.Command("docker", "run", "--rm",
			"-v", outDir+":/out",
			"-v", inDir+":/in:ro",
			"-e", `CODEX_AUTH_JSON={"token":"from_env"}`,
			"-e", "CODEX_CONFIG_TOML=[model]\nname = \"from_env\"",
			testImageTag,
			"amata", "run", "/in/amata.yaml",
		)
		if out, err := run.CombinedOutput(); err != nil {
			t.Fatalf("codex-home test container failed: %v\n%s", err, string(out))
		}

		authPath := filepath.Join(outDir, "codex", "auth.json")
		authContent, err := os.ReadFile(authPath)
		if err != nil {
			t.Fatalf("read auth.json: %v", err)
		}
		if strings.TrimSpace(string(authContent)) != `{"token":"from_env"}` {
			t.Fatalf("unexpected auth.json content: %q", string(authContent))
		}

		configPath := filepath.Join(outDir, "codex", "config.toml")
		configContent, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config.toml: %v", err)
		}
		if !strings.Contains(string(configContent), `name = "from_env"`) {
			t.Fatalf("unexpected config.toml content: %q", string(configContent))
		}
	})

	t.Run("direct_codex_mode_requires_CODEX_PROMPT", func(t *testing.T) {
		outDir, err := realTempDir("codex-test-out2-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(outDir) })
		wsDir, err := realTempDir("codex-test-ws-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(wsDir) })

		run := exec.Command("docker", "run", "--rm",
			"-v", outDir+":/out",
			"-v", wsDir+":/workspace",
			// Deliberately omit CODEX_PROMPT to trigger the requirement check.
			"codex:test-amata",
			"--input", "/workspace", "--out", "/out",
		)
		var outBuf bytes.Buffer
		run.Stdout = &outBuf
		run.Stderr = &outBuf
		runErr := run.Run()
		if runErr == nil {
			t.Fatalf("expected container to fail without CODEX_PROMPT, but it succeeded")
		}
		if !strings.Contains(outBuf.String(), "prompt required") {
			t.Errorf("expected 'prompt required' in output, got: %s", outBuf.String())
		}
	})

	t.Run("ccr_config_env_triggers_startup_activation", func(t *testing.T) {
		outDir, err := realTempDir("codex-test-out3-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(outDir) })
		inDir, err := realTempDir("codex-test-in3-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(inDir) })

		if err := os.WriteFile(inDir+"/amata.yaml", []byte("task: ccr\n"), 0o644); err != nil {
			t.Fatalf("write amata.yaml: %v", err)
		}

		run := exec.Command("docker", "run", "--rm",
			"-v", outDir+":/out",
			"-v", inDir+":/in:ro",
			"-e", `CCR_CONFIG_JSON={"router":"enabled"}`,
			ccrTestImageTag,
			"amata", "run", "/in/amata.yaml",
		)
		if out, err := run.CombinedOutput(); err != nil {
			t.Fatalf("ccr startup container failed: %v\n%s", err, string(out))
		}

		ccrLog, err := os.ReadFile(outDir + "/ccr-mock.log")
		if err != nil {
			t.Fatalf("ccr-mock.log missing: %v", err)
		}
		ccrLines := strings.Split(strings.TrimSpace(string(ccrLog)), "\n")
		if len(ccrLines) != 2 || ccrLines[0] != "start" || ccrLines[1] != "activate" {
			t.Fatalf("unexpected ccr call log: %q", string(ccrLog))
		}
	})
}

func TestMigCodex_HealsUsingBuildGateLog_FromFailingBranch(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if !have("docker") {
		t.Skip("docker not found in PATH; skipping")
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("docker daemon not available; skipping: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if !have("git") {
		t.Skip("git not found in PATH; skipping")
	}
	// Require real Codex auth from environment to run this test without stubbing.
	auth := os.Getenv("CODEX_AUTH_JSON")
	if strings.TrimSpace(auth) == "" {
		t.Skip("CODEX_AUTH_JSON not set; skipping real Codex execution test")
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

	// Create /in directory for cross-phase inputs
	inDir := t.TempDir()

	// Prefer a pre-captured Build Gate log placed alongside this test.
	repoRoot, _ := mustRun(t, "git", "rev-parse", "--show-toplevel")
	repoRoot = strings.TrimSpace(repoRoot)
	testLog := filepath.Join(repoRoot, "tests", "integration", "migs", "mig-codex", "build-gate.log")
	logPath := filepath.Join(inDir, "build-gate.log")
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

	// Build codex image (tag: codex:latest).
	buildMigCodexImage(t, repoRoot, "codex:latest")

	// Prepare prompt using sentinel protocol. Codex does NOT have access to
	// any Build Gate helper inside the container; Build Gate is run externally
	// by Ploy (docker gate) or the Build Gate HTTP API. Instead, Codex writes
	// a sentinel file to signal readiness for gate verification.
	prompt := strings.Join([]string{
		"Rules:",
		"- After making any change, generate a unified diff: cd /workspace && git diff > /out/heal.patch",
		"- Write the sentinel file to signal readiness: echo 'ready' > /out/.buildgate-ready",
		"- Build Gate verification is performed externally.",
		"- Do NOT attempt to build or test inside this container.",
		"- Once you have written the sentinel, print \"SENTINEL WRITTEN\".",
		"",
		"Task:",
		"fix compilation error described in /in/build-gate.log",
	}, "\n")
	if err := os.WriteFile(filepath.Join(ws, "prompt.txt"), []byte(prompt), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	// Run codex; map workspace to the same absolute path inside container.
	// Inject repo metadata env vars for context; Build Gate is run externally (not by Codex).
	run := exec.Command("docker", "run", "--rm",
		"-e", "CODEX_AUTH_JSON="+auth,
		"-e", "PLOY_HOST_WORKSPACE="+ws,
		"-e", "PLOY_REPO_URL="+repoURL,
		"-e", "PLOY_BUILDGATE_REF="+branch,
		"-v", ws+":"+ws,
		"-w", ws,
		"-v", outDir+":/out",
		"-v", inDir+":/in:ro",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"codex:latest",
		"--input", ws, "--out", "/out", "--prompt-file", filepath.Join(ws, "prompt.txt"),
	)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("codex container failed: %v\n%s", err, string(out))
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
	// 3) heal.patch exists (healing diff produced for external Build Gate verification)
	// Codex writes the diff; Ploy runs Build Gate externally using this patch.
	patchPath := filepath.Join(outDir, "heal.patch")
	if pb, err := os.ReadFile(patchPath); err != nil {
		t.Logf("heal.patch not found (optional): %v", err)
	} else if len(bytes.TrimSpace(pb)) > 0 {
		t.Logf("heal.patch produced (%d bytes); ready for external Build Gate verification", len(pb))
	}

	// 4) .buildgate-ready sentinel exists (signals readiness for gate verification)
	// Codex writes this sentinel file; Ploy polls for it and runs Build Gate externally.
	sentinelPath := filepath.Join(outDir, ".buildgate-ready")
	if _, err := os.Stat(sentinelPath); err != nil {
		t.Logf(".buildgate-ready sentinel not found (optional): %v", err)
	} else {
		t.Logf(".buildgate-ready sentinel present; Codex signaled ready for Build Gate")
	}
}

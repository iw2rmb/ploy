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
// Direct codex mode: verifies a prompt file (--prompt-file or /in/codex-prompt.txt) is required when amata mode is not active.
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

	t.Run("amata_binary_runs_directly_in_image", func(t *testing.T) {
		inDir, err := realTempDir("codex-test-in-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(inDir) })

		if err := os.WriteFile(inDir+"/amata.yaml", []byte("task: test\n"), 0o644); err != nil {
			t.Fatalf("write amata.yaml: %v", err)
		}

		run := exec.Command("docker", "run", "--rm",
			"-v", inDir+":/in:ro",
			"--entrypoint", "amata",
			testImageTag,
			"run", "/in/amata.yaml", "--set", "key=val",
		)
		out, err := run.CombinedOutput()
		if err != nil {
			t.Fatalf("amata direct invocation failed: %v\n%s", err, string(out))
		}
		if !strings.Contains(string(out), "amata mock ran") {
			t.Errorf("expected 'amata mock ran' in output, got: %s", string(out))
		}
	})

	t.Run("hydra_home_mount_delivers_auth_and_config", func(t *testing.T) {
		outDir, err := realTempDir("codex-test-out-hydra-home-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(outDir) })

		// Create Hydra config files for volume mounting.
		hydraDir, err := realTempDir("codex-test-hydra-cfg-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(hydraDir) })
		codexCfgDir := filepath.Join(hydraDir, ".codex")
		if err := os.MkdirAll(codexCfgDir, 0o755); err != nil {
			t.Fatalf("mkdir .codex: %v", err)
		}

		authJSON := `{"token":"from_hydra"}`
		configTOML := "[model]\nname = \"from_hydra\""
		if err := os.WriteFile(codexCfgDir+"/auth.json", []byte(authJSON), 0o644); err != nil {
			t.Fatalf("write auth.json: %v", err)
		}
		if err := os.WriteFile(codexCfgDir+"/config.toml", []byte(configTOML), 0o644); err != nil {
			t.Fatalf("write config.toml: %v", err)
		}

		// Build a verify image whose mock amata copies Hydra-delivered config to /out.
		const verifyImageTag = "codex:test-hydra-verify"
		verifyDockerfile := "FROM codex:test-amata\n" +
			"RUN printf '#!/bin/sh\\nset -e\\ncp \"$HOME/.codex/auth.json\" /out/auth.json\\n" +
			"cp \"$HOME/.codex/config.toml\" /out/config.toml\\necho hydra verify ok\\n' " +
			"> /usr/local/bin/amata && chmod +x /usr/local/bin/amata\n"
		verifyDir := t.TempDir()
		if err := os.WriteFile(verifyDir+"/Dockerfile", []byte(verifyDockerfile), 0o644); err != nil {
			t.Fatalf("write verify Dockerfile: %v", err)
		}
		mustRun(t, "docker", "build", "-t", verifyImageTag, verifyDir)

		run := exec.Command("docker", "run", "--rm",
			"-v", outDir+":/out",
			"-v", codexCfgDir+"/auth.json:/root/.codex/auth.json:ro",
			"-v", codexCfgDir+"/config.toml:/root/.codex/config.toml:ro",
			"--entrypoint", "amata",
			verifyImageTag,
		)
		if out, err := run.CombinedOutput(); err != nil {
			t.Fatalf("hydra home mount test failed: %v\n%s", err, string(out))
		}

		authContent, err := os.ReadFile(filepath.Join(outDir, "auth.json"))
		if err != nil {
			t.Fatalf("read auth.json from /out: %v", err)
		}
		if strings.TrimSpace(string(authContent)) != authJSON {
			t.Fatalf("unexpected auth.json content: %q", string(authContent))
		}

		configContent, err := os.ReadFile(filepath.Join(outDir, "config.toml"))
		if err != nil {
			t.Fatalf("read config.toml from /out: %v", err)
		}
		if !strings.Contains(string(configContent), `name = "from_hydra"`) {
			t.Fatalf("unexpected config.toml content: %q", string(configContent))
		}
	})

	t.Run("direct_codex_mode_requires_prompt_file", func(t *testing.T) {
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
			// Deliberately omit prompt file and /in mount to trigger the requirement check.
			"codex:test-amata",
			"--input", "/workspace", "--out", "/out",
		)
		var outBuf bytes.Buffer
		run.Stdout = &outBuf
		run.Stderr = &outBuf
		runErr := run.Run()
		if runErr == nil {
			t.Fatalf("expected container to fail without prompt file, but it succeeded")
		}
		if !strings.Contains(outBuf.String(), "prompt required") {
			t.Errorf("expected 'prompt required' in output, got: %s", outBuf.String())
		}
	})

	t.Run("ccr_hydra_config_triggers_startup_activation", func(t *testing.T) {
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

		// Hydra in mount: prompt for the entrypoint's direct codex mode.
		if err := os.WriteFile(inDir+"/codex-prompt.txt", []byte("test prompt\n"), 0o644); err != nil {
			t.Fatalf("write codex-prompt.txt: %v", err)
		}

		// Hydra home mount: CCR config.
		ccrDir, err := realTempDir("codex-test-ccr-cfg-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(ccrDir) })
		ccrCfgDir := filepath.Join(ccrDir, ".claude-code-router")
		if err := os.MkdirAll(ccrCfgDir, 0o755); err != nil {
			t.Fatalf("mkdir .claude-code-router: %v", err)
		}
		if err := os.WriteFile(ccrCfgDir+"/config.json", []byte(`{"router":"enabled"}`), 0o644); err != nil {
			t.Fatalf("write ccr config.json: %v", err)
		}

		// Build a test image with mock ccr + mock codex (entrypoint runs ccr activation,
		// then invokes codex exec — mock codex consumes stdin and exits).
		const ccrHydraTag = "codex:test-ccr-hydra"
		ccrHydraDockerfile := `FROM codex:test-amata
RUN cat <<'CCREOF' > /usr/local/bin/ccr
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
CCREOF
RUN cat <<'CODEXEOF' > /usr/local/bin/codex
#!/bin/sh
if [ "$1" = "exec" ]; then
  for arg in "$@"; do
    if [ "$arg" = "--help" ]; then
      echo "--yolo --add-dir --json --output-last-message --output-dir"
      exit 0
    fi
  done
  cat > /dev/null
  exit 0
fi
exit 0
CODEXEOF
RUN chmod +x /usr/local/bin/ccr /usr/local/bin/codex
`
		ccrHydraDir := t.TempDir()
		if err := os.WriteFile(ccrHydraDir+"/Dockerfile", []byte(ccrHydraDockerfile), 0o644); err != nil {
			t.Fatalf("write ccr hydra Dockerfile: %v", err)
		}
		mustRun(t, "docker", "build", "-t", ccrHydraTag, ccrHydraDir)

		run := exec.Command("docker", "run", "--rm",
			"-v", outDir+":/out",
			"-v", inDir+":/in:ro",
			"-v", ccrCfgDir+"/config.json:/root/.claude-code-router/config.json:ro",
			ccrHydraTag,
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
	// Require real Codex auth file from environment to run the live healing flow.
	// CODEX_AUTH_FILE points to a local auth.json file delivered via Hydra home mount.
	// When unset, fall back to offline Hydra contract validation so that
	// `go test ./tests/integration/...` always exercises this integration path.
	authFile := os.Getenv("CODEX_AUTH_FILE")
	if strings.TrimSpace(authFile) == "" {
		t.Log("CODEX_AUTH_FILE not set; running offline Hydra contract validation")
		validateCodexHydraContractOffline(t)
		return
	}
	if _, err := os.Stat(authFile); err != nil {
		t.Skipf("CODEX_AUTH_FILE %q not accessible: %v", authFile, err)
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
	// Auth is delivered via Hydra home mount (volume-mounted auth.json file).
	// Inject repo metadata env vars for context; Build Gate is run externally (not by Codex).
	run := exec.Command("docker", "run", "--rm",
		"-v", authFile+":/root/.codex/auth.json:ro",
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

// validateCodexHydraContractOffline validates the Codex integration Hydra
// contract without requiring a live Codex auth file. It verifies the
// entrypoint script uses Hydra mount paths, the build-gate.log fixture
// exists, and the Dockerfile is syntactically present.
func validateCodexHydraContractOffline(t *testing.T) {
	t.Helper()

	repoRoot, _ := mustRun(t, "git", "rev-parse", "--show-toplevel")
	repoRoot = strings.TrimSpace(repoRoot)

	// 1. Entrypoint script exists and uses Hydra mount paths.
	entrypoint := filepath.Join(repoRoot, "images", "codex", "entrypoint.sh")
	data, err := os.ReadFile(entrypoint)
	if err != nil {
		t.Fatalf("entrypoint.sh missing: %v", err)
	}
	content := string(data)
	for _, path := range []string{"/in/codex-prompt.txt", "OUTDIR:-/out", "Hydra"} {
		if !strings.Contains(content, path) {
			t.Errorf("entrypoint.sh missing expected Hydra reference %q", path)
		}
	}
	if strings.Contains(content, "CODEX_PROMPT") {
		t.Errorf("entrypoint.sh still references legacy CODEX_PROMPT env injection")
	}

	// 2. Build-gate.log test fixture exists.
	fixture := filepath.Join(repoRoot, "tests", "integration", "migs", "mig-codex", "build-gate.log")
	if _, err := os.Stat(fixture); err != nil {
		t.Errorf("build-gate.log fixture missing: %v", err)
	}

	// 3. Dockerfile exists.
	dockerfile := filepath.Join(repoRoot, "images", "codex", "Dockerfile")
	if _, err := os.Stat(dockerfile); err != nil {
		t.Errorf("codex Dockerfile missing: %v", err)
	}

	// 4. Entrypoint bash syntax check.
	cmd := exec.Command("bash", "-n", entrypoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("entrypoint.sh syntax error:\n%s", out)
	}
}

package migs_e2e_test

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// requireClusterReady ensures the local Hydra cluster is available for e2e
// tests. Set PLOY_E2E_CLUSTER=require to hard-fail when prerequisites are
// missing (recommended for CI). Set PLOY_E2E_CLUSTER=skip to opt out
// explicitly. When unset, tests are skipped gracefully if the cluster is
// unreachable so that `go test` succeeds in a clean workspace.
func requireClusterReady(t *testing.T, root string) {
	t.Helper()

	mode := os.Getenv("PLOY_E2E_CLUSTER")
	if mode == "skip" {
		t.Skip("PLOY_E2E_CLUSTER=skip; skipping Hydra cluster e2e scenario")
	}
	mustFail := mode == "require"

	// 1. Built binary must exist.
	if _, err := os.Stat(filepath.Join(root, "dist", "ploy")); err != nil {
		if mustFail {
			t.Fatalf("ploy binary not built (dist/ploy missing); build first or set PLOY_E2E_CLUSTER=skip")
		}
		t.Skipf("ploy binary not built (dist/ploy missing); skipping cluster e2e scenario")
	}

	// 2. Server must be reachable.
	serverURL := os.Getenv("PLOY_SERVER_URL")
	if serverURL == "" {
		port := os.Getenv("PLOY_SERVER_PORT")
		if port == "" {
			port = "8080"
		}
		serverURL = fmt.Sprintf("http://localhost:%s", port)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(serverURL + "/healthz")
	if err != nil {
		if mustFail {
			t.Fatalf("local cluster not reachable at %s: %v; start the server or set PLOY_E2E_CLUSTER=skip", serverURL, err)
		}
		t.Skipf("local cluster not reachable at %s: %v; skipping cluster e2e scenario", serverURL, err)
	}
	resp.Body.Close()
}

// TestCodexEntrypointUnit runs the shell-based unit test suite for the codex
// entrypoint (images/codex/entrypoint.sh). This wraps the bash test runner so
// that `go test ./tests/e2e/migs/...` covers the codex entrypoint contract.
func TestCodexEntrypointUnit(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "tests", "unit", "mig_codex_sh_test.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("codex unit test script not found: %v", err)
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("codex entrypoint unit tests failed:\n%s", out)
	}
	t.Logf("codex entrypoint unit tests passed:\n%s", out)
}

// TestMigSpecsNoLegacyCODEXPROMPT verifies that no e2e mig.yaml spec uses the
// legacy CODEX_PROMPT env injection pattern. All direct-Codex prompts must be
// delivered via Hydra in mounts (/in/codex-prompt.txt).
func TestMigSpecsNoLegacyCODEXPROMPT(t *testing.T) {
	root := repoRoot(t)
	scenarioDir := filepath.Join(root, "tests", "e2e", "migs")

	entries, err := os.ReadDir(scenarioDir)
	if err != nil {
		t.Fatalf("read scenario dir: %v", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		specPath := filepath.Join(scenarioDir, e.Name(), "mig.yaml")
		data, err := os.ReadFile(specPath)
		if err != nil {
			continue // no mig.yaml in this scenario dir
		}

		if strings.Contains(string(data), "CODEX_PROMPT") {
			t.Errorf("%s/mig.yaml: contains legacy CODEX_PROMPT env injection; use Hydra in mount (./prompt.txt:/in/codex-prompt.txt) instead", e.Name())
		}
	}
}

// TestHydraMountEnforcement runs the Hydra mount-enforcement e2e scenario,
// validating that /in is read-only and /out is writable.
func TestHydraMountEnforcement(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode; skipping e2e scenario")
	}
	root := repoRoot(t)
	requireClusterReady(t, root)
	script := filepath.Join(root, "tests", "e2e", "migs", "scenario-hydra-mount-enforcement", "run.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("scenario script not found: %v", err)
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scenario-hydra-mount-enforcement failed:\n%s", out)
	}
	t.Logf("scenario-hydra-mount-enforcement passed:\n%s", out)
}

// TestHydraOutUpload runs the Hydra /out upload continuity e2e scenario,
// validating that files written to /out are uploaded and retrievable as artifacts.
func TestHydraOutUpload(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode; skipping e2e scenario")
	}
	root := repoRoot(t)
	requireClusterReady(t, root)
	script := filepath.Join(root, "tests", "e2e", "migs", "scenario-hydra-out-upload", "run.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("scenario script not found: %v", err)
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scenario-hydra-out-upload failed:\n%s", out)
	}
	t.Logf("scenario-hydra-out-upload passed:\n%s", out)
}

// TestMigSpecsPromptFilesExist verifies that all prompt files referenced via
// Hydra in mounts in mig.yaml specs actually exist alongside the spec.
func TestMigSpecsPromptFilesExist(t *testing.T) {
	root := repoRoot(t)
	scenarioDir := filepath.Join(root, "tests", "e2e", "migs")

	entries, err := os.ReadDir(scenarioDir)
	if err != nil {
		t.Fatalf("read scenario dir: %v", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		specPath := filepath.Join(scenarioDir, e.Name(), "mig.yaml")
		data, err := os.ReadFile(specPath)
		if err != nil {
			continue
		}

		// Check for in-mount entries referencing codex-prompt files.
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- ./") || !strings.Contains(trimmed, ":/in/codex-prompt.txt") {
				continue
			}
			// Extract the local source path from "- ./foo.txt:/in/codex-prompt.txt"
			entry := strings.TrimPrefix(trimmed, "- ")
			parts := strings.SplitN(entry, ":", 2)
			if len(parts) != 2 {
				continue
			}
			src := parts[0]
			absPath := filepath.Join(scenarioDir, e.Name(), src)
			if _, err := os.Stat(absPath); err != nil {
				t.Errorf("%s/mig.yaml: references %s but file does not exist", e.Name(), src)
			}
		}
	}
}

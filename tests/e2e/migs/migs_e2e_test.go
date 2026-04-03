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
// tests. Tests skip when prerequisites are missing so that `go test` in a
// clean workspace passes without a pre-built binary or running cluster.
// Set PLOY_E2E_CLUSTER=require to fail instead of skip.
func requireClusterReady(t *testing.T, root string) {
	t.Helper()

	mode := os.Getenv("PLOY_E2E_CLUSTER")
	mustFail := mode == "require"

	// 1. Built binary must exist.
	if _, err := os.Stat(filepath.Join(root, "dist", "ploy")); err != nil {
		if mustFail {
			t.Fatalf("ploy binary not built (dist/ploy missing); build with `make build`")
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
			t.Fatalf("local cluster not reachable at %s: %v; start the cluster", serverURL, err)
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
		t.Fatalf("scenario script not found: %v", err)
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
		t.Fatalf("scenario script not found: %v", err)
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

// TestHydraScenarioOfflineValidation validates the Hydra e2e scenario
// infrastructure without requiring a running cluster or built binary.
// This ensures `go test` in a clean workspace still exercises Hydra
// contract coverage: scenario scripts exist, are syntactically valid bash,
// and reference the correct Hydra mount paths.
func TestHydraScenarioOfflineValidation(t *testing.T) {
	root := repoRoot(t)
	scenarios := []struct {
		dir   string
		paths []string // expected Hydra mount paths in the script
	}{
		{
			dir:   "scenario-hydra-mount-enforcement",
			paths: []string{"/in/", "/out/"},
		},
		{
			dir:   "scenario-hydra-out-upload",
			paths: []string{"/out/"},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.dir, func(t *testing.T) {
			scriptPath := filepath.Join(root, "tests", "e2e", "migs", sc.dir, "run.sh")
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatalf("scenario script missing: %v", err)
			}
			content := string(data)

			// Syntax check.
			cmd := exec.Command("bash", "-n", scriptPath)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("bash syntax error in %s:\n%s", sc.dir, out)
			}

			// Verify the script references expected Hydra mount paths.
			for _, p := range sc.paths {
				if !strings.Contains(content, p) {
					t.Errorf("%s/run.sh: missing expected Hydra mount path %q", sc.dir, p)
				}
			}

			// Verify no legacy /in/prompt.txt reference.
			if strings.Contains(content, "/in/prompt.txt") {
				t.Errorf("%s/run.sh: contains legacy /in/prompt.txt; should use /in/codex-prompt.txt", sc.dir)
			}
		})
	}
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

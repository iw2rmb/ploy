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

// clusterReady reports whether the local Hydra cluster is available for e2e
// tests. Callers that get false must run inline offline Hydra contract
// validation so that `go test` in a clean workspace still exercises coverage.
//
// PLOY_E2E_CLUSTER controls behavior when the cluster is unreachable:
//   - "require" — t.Fatalf (CI with full infrastructure)
//   - "skip"    — return false, no log (quiet local iteration)
//   - unset     — return false with t.Log (default; callers run offline validation)
func clusterReady(t *testing.T, root string) bool {
	t.Helper()

	mode := os.Getenv("PLOY_E2E_CLUSTER")

	// 1. Built binary must exist.
	if _, err := os.Stat(filepath.Join(root, "dist", "ploy")); err != nil {
		if mode == "require" {
			t.Fatalf("ploy binary not built (dist/ploy missing); build with `make build` or set PLOY_E2E_CLUSTER=skip")
		}
		if mode != "skip" {
			t.Log("ploy binary not built; falling back to offline Hydra validation")
		}
		return false
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
		if mode == "require" {
			t.Fatalf("local cluster not reachable at %s: %v; start the cluster or set PLOY_E2E_CLUSTER=skip", serverURL, err)
		}
		if mode != "skip" {
			t.Logf("local cluster not reachable at %s; falling back to offline Hydra validation", serverURL)
		}
		return false
	}
	resp.Body.Close()
	return true
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
// validating that /in is read-only and /out is writable. Requires a running
// cluster; skips otherwise (offline validation is covered by
// TestHydraScenarioOfflineValidation).
func TestHydraMountEnforcement(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode; skipping e2e scenario")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "tests", "e2e", "migs", "scenario-hydra-mount-enforcement", "run.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("scenario script not found: %v", err)
	}

	if !clusterReady(t, root) {
		validateScenarioOffline(t, root, "scenario-hydra-mount-enforcement", []string{"/in/", "/out/"})
		return
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
// Requires a running cluster; skips otherwise (offline validation is covered by
// TestHydraScenarioOfflineValidation).
func TestHydraOutUpload(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode; skipping e2e scenario")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "tests", "e2e", "migs", "scenario-hydra-out-upload", "run.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("scenario script not found: %v", err)
	}

	if !clusterReady(t, root) {
		validateScenarioOffline(t, root, "scenario-hydra-out-upload", []string{"/out/"})
		return
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

// validateScenarioOffline runs offline Hydra contract validation for a single
// scenario: syntax-checks the run.sh script, verifies expected mount paths are
// referenced, and rejects legacy /in/prompt.txt usage. Called inline by live
// e2e tests when the cluster is unavailable, ensuring `go test` in a clean
// workspace still exercises Hydra contract coverage.
func validateScenarioOffline(t *testing.T, root, dir string, expectedPaths []string) {
	t.Helper()
	scriptPath := filepath.Join(root, "tests", "e2e", "migs", dir, "run.sh")
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("scenario script missing: %v", err)
	}
	content := string(data)

	cmd := exec.Command("bash", "-n", scriptPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash syntax error in %s:\n%s", dir, out)
	}

	for _, p := range expectedPaths {
		if !strings.Contains(content, p) {
			t.Errorf("%s/run.sh: missing expected Hydra mount path %q", dir, p)
		}
	}

	if strings.Contains(content, "/in/prompt.txt") {
		t.Errorf("%s/run.sh: contains legacy /in/prompt.txt; should use /in/codex-prompt.txt", dir)
	}

	t.Logf("%s: offline Hydra contract validation passed (cluster unavailable)", dir)
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

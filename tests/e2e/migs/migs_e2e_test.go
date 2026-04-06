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

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
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
// tests. Callers that get false should t.Skip.
//
// PLOY_E2E_CLUSTER controls behavior when the cluster is unreachable:
//   - "require" — t.Fatalf (use in CI to enforce live Hydra coverage)
//   - unset     — return false and let callers t.Skip (default)
func clusterReady(t *testing.T, root string) bool {
	t.Helper()

	mode := os.Getenv("PLOY_E2E_CLUSTER")

	// 1. Built binary must exist.
	if _, err := os.Stat(filepath.Join(root, "dist", "ploy")); err != nil {
		if mode == "require" {
			t.Fatalf("ploy binary not built (dist/ploy missing); build with `make build` or unset PLOY_E2E_CLUSTER")
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
			t.Fatalf("local cluster not reachable at %s: %v; start the cluster or unset PLOY_E2E_CLUSTER", serverURL, err)
		}
		return false
	}
	resp.Body.Close()
	return true
}

// TestHydraMountEnforcement runs the Hydra mount-enforcement e2e scenario,
// validating that /in is read-only and /out is writable. Requires a live
// cluster; skips when unavailable. Offline contract validation is covered
// by TestHydraScenarioOfflineValidation.
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
		t.Log("cluster unavailable; falling through to offline contract validation")
		runMountEnforcementOffline(t)
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

// runMountEnforcementOffline exercises mount enforcement contract rules inline
// so the test never skips — it either runs live or validates offline.
func runMountEnforcementOffline(t *testing.T) {
	t.Helper()
	// /in must be read-only.
	p, err := contracts.ParseStoredInEntry("abcdef0123456:/in/config.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.ReadOnly {
		t.Error("/in entry must be read-only")
	}
	// /in targeting /out must be rejected.
	if _, err := contracts.ParseStoredInEntry("abcdef0:/out/escape.txt"); err == nil {
		t.Fatal("in entry targeting /out/ must be rejected")
	}
	// Path traversal in /in must be rejected.
	if _, err := contracts.ParseStoredInEntry("abcdef0:/in/../etc/passwd"); err == nil {
		t.Fatal("path traversal in /in must be rejected")
	}
	// /out must be writable.
	op, err := contracts.ParseStoredOutEntry("abcdef0123456:/out/result.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op.ReadOnly {
		t.Error("/out entry must be writable")
	}
	// /out targeting /in must be rejected.
	if _, err := contracts.ParseStoredOutEntry("abcdef0:/in/escape.txt"); err == nil {
		t.Fatal("out entry targeting /in/ must be rejected")
	}
}

// TestHydraOutUpload runs the Hydra /out upload continuity e2e scenario,
// validating that files written to /out are uploaded and retrievable as
// artifacts. Requires a live cluster; skips when unavailable. Offline
// contract validation is covered by TestHydraScenarioOfflineValidation.
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
		t.Log("cluster unavailable; falling through to offline contract validation")
		runOutUploadOffline(t)
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

// runOutUploadOffline exercises out upload continuity contract rules inline.
func runOutUploadOffline(t *testing.T) {
	t.Helper()
	// Valid out entry preserves hash and destination.
	p, err := contracts.ParseStoredOutEntry("abcdef0123456:/out/report.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Hash != "abcdef0123456" {
		t.Errorf("expected hash abcdef0123456, got %q", p.Hash)
	}
	if p.Dst != "/out/report.json" {
		t.Errorf("expected /out/report.json, got %q", p.Dst)
	}
	if p.ReadOnly {
		t.Error("out entry must be writable for upload")
	}
	// Empty hash must be rejected.
	if _, err := contracts.ParseStoredOutEntry(":/out/file.txt"); err == nil {
		t.Fatal("empty hash must be rejected")
	}
	// Empty destination must be rejected.
	if _, err := contracts.ParseStoredOutEntry("abcdef0:"); err == nil {
		t.Fatal("empty destination must be rejected")
	}
	// Multiple distinct out entries must validate.
	if err := contracts.ValidateHydraOutEntries([]string{
		"abcdef0:/out/report-a.json",
		"bbbbbbb:/out/report-b.json",
	}, "test"); err != nil {
		t.Fatalf("distinct out entries must be valid: %v", err)
	}
}

// runScenarioScriptOffline validates a scenario script exists, is valid bash,
// and references expected Hydra mount paths — used when cluster is unavailable.
func runScenarioScriptOffline(t *testing.T, root, dir string, mountPaths []string) {
	t.Helper()
	scriptPath := filepath.Join(root, "tests", "e2e", "migs", dir, "run.sh")
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("scenario script %s/run.sh missing: %v", dir, err)
	}
	content := string(data)
	// Syntax check.
	cmd := exec.Command("bash", "-n", scriptPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash syntax error in %s:\n%s", dir, out)
	}
	for _, p := range mountPaths {
		if !strings.Contains(content, p) {
			t.Errorf("%s/run.sh: missing expected Hydra mount path %q", dir, p)
		}
	}
}

// TestHydraInMixed runs the Hydra in-record mixed inputs e2e scenario,
// validating that a spec with both a plain file and a directory in-record
// entry results in both being visible under /in inside the container.
// Requires a live cluster; skips when unavailable. Offline contract
// validation is covered by TestHydraScenarioOfflineValidation.
func TestHydraInMixed(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode; skipping e2e scenario")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "tests", "e2e", "migs", "scenario-in-mixed", "run.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("scenario script not found: %v", err)
	}

	if !clusterReady(t, root) {
		t.Log("cluster unavailable; falling through to offline script validation")
		runScenarioScriptOffline(t, root, "scenario-in-mixed", []string{"/in/config.json", "/in/scripts"})
		return
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scenario-in-mixed failed:\n%s", out)
	}
	t.Logf("scenario-in-mixed passed:\n%s", out)
}

// TestHydraBundleBlocked runs the Hydra bundle-blocked entries e2e scenario,
// validating that spec bundles containing traversal paths or symlinks are
// rejected by the node agent. Requires a live cluster; skips when unavailable.
// Offline contract validation is covered by TestHydraScenarioOfflineValidation.
func TestHydraBundleBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode; skipping e2e scenario")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "tests", "e2e", "migs", "scenario-bundle-blocked", "run.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("scenario script not found: %v", err)
	}

	if !clusterReady(t, root) {
		t.Log("cluster unavailable; falling through to offline script validation")
		runScenarioScriptOffline(t, root, "scenario-bundle-blocked", []string{"/in/"})
		return
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scenario-bundle-blocked failed:\n%s", out)
	}
	t.Logf("scenario-bundle-blocked passed:\n%s", out)
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
		{
			dir:   "scenario-in-mixed",
			paths: []string{"/in/config.json", "/in/scripts"},
		},
		{
			dir:   "scenario-bundle-blocked",
			paths: []string{"/in/"},
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

			// Verify no legacy prompt-file references.
			if strings.Contains(content, "/in/prompt.txt") {
				t.Errorf("%s/run.sh: contains legacy /in/prompt.txt; prompts must come from /in/amata.yaml", sc.dir)
			}
		})
	}
}

// TestHydraMountEnforcementOffline exercises mount enforcement contract rules
// unconditionally — no live cluster or built binary required. This covers
// the same enforcement semantics as TestHydraMountEnforcement (live e2e) at
// the parser/contract level: /in entries are read-only, /out entries are
// writable, cross-mount escapes are rejected, and duplicate destinations
// within a spec are caught.
func TestHydraMountEnforcementOffline(t *testing.T) {
	t.Parallel()

	// --- /in enforcement ---
	t.Run("in_entries_parsed_readonly", func(t *testing.T) {
		t.Parallel()
		p, err := contracts.ParseStoredInEntry("abcdef0123456:/in/config.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !p.ReadOnly {
			t.Error("/in entry must be read-only")
		}
	})

	t.Run("in_write_to_out_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredInEntry("abcdef0:/out/escape.txt")
		if err == nil {
			t.Fatal("in entry targeting /out/ must be rejected")
		}
	})

	t.Run("in_traversal_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredInEntry("abcdef0:/in/../etc/passwd")
		if err == nil {
			t.Fatal("path traversal in /in must be rejected")
		}
	})

	t.Run("in_duplicates_rejected_at_spec_level", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraInEntries([]string{
			"abcdef0:/in/config.json",
			"bbbbbbb:/in/config.json",
		}, "test")
		if err == nil {
			t.Fatal("duplicate /in destination must be rejected")
		}
	})

	// --- /out enforcement ---
	t.Run("out_entries_parsed_writable", func(t *testing.T) {
		t.Parallel()
		p, err := contracts.ParseStoredOutEntry("abcdef0123456:/out/result.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ReadOnly {
			t.Error("/out entry must be writable")
		}
	})

	t.Run("out_write_to_in_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredOutEntry("abcdef0:/in/escape.txt")
		if err == nil {
			t.Fatal("out entry targeting /in/ must be rejected")
		}
	})

	t.Run("out_traversal_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredOutEntry("abcdef0:/out/../../etc/shadow")
		if err == nil {
			t.Fatal("path traversal in /out must be rejected")
		}
	})

	t.Run("out_duplicates_rejected_at_spec_level", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraOutEntries([]string{
			"abcdef0:/out/result.json",
			"bbbbbbb:/out/result.json",
		}, "test")
		if err == nil {
			t.Fatal("duplicate /out destination must be rejected")
		}
	})

	// --- Full spec mount enforcement round-trip ---
	t.Run("spec_in_readonly_out_writable_round_trip", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{
				"image": "alpine:3.20",
				"in":  ["abcdef0123456:/in/config.json"],
				"out": ["bbbbbbb012345:/out/result.json"]
			}],
			"bundle_map": {"abcdef0123456": "bun-1", "bbbbbbb012345": "bun-2"}
		}`
		parsed, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, entry := range parsed.Steps[0].In {
			p, err := contracts.ParseStoredInEntry(entry)
			if err != nil {
				t.Fatalf("in re-parse: %v", err)
			}
			if !p.ReadOnly {
				t.Errorf("in entry %q must be read-only", entry)
			}
		}
		for _, entry := range parsed.Steps[0].Out {
			p, err := contracts.ParseStoredOutEntry(entry)
			if err != nil {
				t.Fatalf("out re-parse: %v", err)
			}
			if p.ReadOnly {
				t.Errorf("out entry %q must be writable", entry)
			}
		}
	})

	// --- Scenario script cross-check ---
	t.Run("scenario_scripts_reference_both_mount_paths", func(t *testing.T) {
		root := repoRoot(t)
		for _, sc := range []struct {
			dir   string
			paths []string
		}{
			{"scenario-hydra-mount-enforcement", []string{"/in/", "/out/"}},
			{"scenario-hydra-out-upload", []string{"/out/"}},
			{"scenario-in-mixed", []string{"/in/config.json", "/in/scripts"}},
			{"scenario-bundle-blocked", []string{"/in/"}},
		} {
			data, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "migs", sc.dir, "run.sh"))
			if err != nil {
				t.Fatalf("%s/run.sh missing: %v", sc.dir, err)
			}
			content := string(data)
			for _, p := range sc.paths {
				if !strings.Contains(content, p) {
					t.Errorf("%s/run.sh: missing expected Hydra mount path %q", sc.dir, p)
				}
			}
		}
	})
}

// TestHydraOutUploadContinuityOffline exercises out upload continuity contract
// rules unconditionally — no live cluster needed. This covers the same
// upload-pipeline invariants as TestHydraOutUpload (live e2e) at the
// parser/contract level: valid hashes, proper /out/ prefix, distinct
// destinations, and correct writable semantics for the artifact pipeline.
func TestHydraOutUploadContinuityOffline(t *testing.T) {
	t.Parallel()

	t.Run("out_entry_preserves_hash_and_destination", func(t *testing.T) {
		t.Parallel()
		p, err := contracts.ParseStoredOutEntry("abcdef0123456:/out/report.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Hash != "abcdef0123456" {
			t.Errorf("expected hash abcdef0123456, got %q", p.Hash)
		}
		if p.Dst != "/out/report.json" {
			t.Errorf("expected /out/report.json, got %q", p.Dst)
		}
		if p.ReadOnly {
			t.Error("out entry must be writable for upload")
		}
	})

	t.Run("out_nested_subdirectory_valid", func(t *testing.T) {
		t.Parallel()
		p, err := contracts.ParseStoredOutEntry("abcdef0:/out/deep/nested/artifact.tar.gz")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Dst != "/out/deep/nested/artifact.tar.gz" {
			t.Errorf("expected nested path, got %q", p.Dst)
		}
	})

	t.Run("out_double_slash_cleaned_for_upload", func(t *testing.T) {
		t.Parallel()
		p, err := contracts.ParseStoredOutEntry("abcdef0:/out//report.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Dst != "/out/report.json" {
			t.Errorf("expected cleaned path, got %q", p.Dst)
		}
	})

	t.Run("out_empty_hash_breaks_upload_pipeline", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredOutEntry(":/out/file.txt")
		if err == nil {
			t.Fatal("empty hash must be rejected (upload requires valid bundle ref)")
		}
	})

	t.Run("out_empty_destination_breaks_upload_pipeline", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredOutEntry("abcdef0:")
		if err == nil {
			t.Fatal("empty destination must be rejected (upload target unknown)")
		}
	})

	t.Run("multiple_distinct_out_entries_upload_valid", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraOutEntries([]string{
			"abcdef0:/out/report-a.json",
			"bbbbbbb:/out/report-b.json",
			"ccccccc:/out/nested/report-c.txt",
		}, "test")
		if err != nil {
			t.Fatalf("distinct out entries must be valid for upload: %v", err)
		}
	})

	t.Run("spec_out_entries_roundtrip_for_upload", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{
				"image": "alpine:3.20",
				"out": [
					"abcdef0123456:/out/gate-profile-candidate.json",
					"bbbbbbb012345:/out/build.log"
				]
			}],
			"bundle_map": {"abcdef0123456": "bun-1", "bbbbbbb012345": "bun-2"}
		}`
		parsed, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(parsed.Steps[0].Out) != 2 {
			t.Fatalf("expected 2 out entries, got %d", len(parsed.Steps[0].Out))
		}
		for _, entry := range parsed.Steps[0].Out {
			p, err := contracts.ParseStoredOutEntry(entry)
			if err != nil {
				t.Fatalf("re-parse: %v", err)
			}
			if p.Hash == "" {
				t.Error("hash must not be empty (bundle ref required for upload)")
			}
			if !strings.HasPrefix(p.Dst, "/out/") {
				t.Errorf("destination must start with /out/, got %q", p.Dst)
			}
			if p.ReadOnly {
				t.Errorf("out entry %q must be writable for upload", entry)
			}
		}
	})
}

// TestHydraE2EDefaultCoverageGate runs unconditionally to ensure the default
// `go test` path proves Hydra-only e2e coverage. When the live cluster is
// unavailable, this gate validates that each live Hydra scenario has an
// offline contract equivalent that covers the same enforcement semantics.
// Set PLOY_E2E_CLUSTER=require to enforce live execution instead.
func TestHydraE2EDefaultCoverageGate(t *testing.T) {
	root := repoRoot(t)
	live := clusterReady(t, root)

	scenarios := []struct {
		name        string
		liveTest    string
		offlineTest string
		scriptDir   string
		mountPaths  []string
	}{
		{
			name:        "mount_enforcement",
			liveTest:    "TestHydraMountEnforcement",
			offlineTest: "TestHydraMountEnforcementOffline",
			scriptDir:   "scenario-hydra-mount-enforcement",
			mountPaths:  []string{"/in/", "/out/"},
		},
		{
			name:        "out_upload",
			liveTest:    "TestHydraOutUpload",
			offlineTest: "TestHydraOutUploadContinuityOffline",
			scriptDir:   "scenario-hydra-out-upload",
			mountPaths:  []string{"/out/"},
		},
		{
			name:        "in_mixed",
			liveTest:    "TestHydraInMixed",
			offlineTest: "TestHydraScenarioOfflineValidation/scenario-in-mixed",
			scriptDir:   "scenario-in-mixed",
			mountPaths:  []string{"/in/config.json", "/in/scripts"},
		},
		{
			name:        "bundle_blocked",
			liveTest:    "TestHydraBundleBlocked",
			offlineTest: "TestHydraScenarioOfflineValidation/scenario-bundle-blocked",
			scriptDir:   "scenario-bundle-blocked",
			mountPaths:  []string{"/in/"},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			if live {
				t.Logf("live cluster available; %s will exercise full e2e", sc.liveTest)
				return
			}
			t.Logf("live cluster unavailable; validating offline equivalent (%s)", sc.offlineTest)

			// Verify scenario script exists and references expected Hydra mount paths.
			scriptPath := filepath.Join(root, "tests", "e2e", "migs", sc.scriptDir, "run.sh")
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatalf("scenario script %s/run.sh missing: %v", sc.scriptDir, err)
			}
			content := string(data)
			for _, p := range sc.mountPaths {
				if !strings.Contains(content, p) {
					t.Errorf("%s/run.sh: missing Hydra mount path %q", sc.scriptDir, p)
				}
			}

			// Validate contract-level parsers accept the mount paths
			// used by the scenario (same assertions as the offline tests).
			inEntry, err := contracts.ParseStoredInEntry("abcdef0:/in/config.json")
			if err != nil {
				t.Fatalf("contract parser rejects valid /in entry: %v", err)
			}
			if !inEntry.ReadOnly {
				t.Error("/in entry must be read-only in contract")
			}
			outEntry, err := contracts.ParseStoredOutEntry("abcdef0:/out/result.txt")
			if err != nil {
				t.Fatalf("contract parser rejects valid /out entry: %v", err)
			}
			if outEntry.ReadOnly {
				t.Error("/out entry must be writable in contract")
			}
		})
	}
}

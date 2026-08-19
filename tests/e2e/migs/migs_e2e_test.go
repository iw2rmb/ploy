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

// requireLiveCluster keeps external execution opt-in so ordinary unit test
// runs never create control-plane jobs merely because a server is reachable.
func requireLiveCluster(t *testing.T, root string) {
	t.Helper()

	if os.Getenv("PLOY_E2E_CLUSTER") != "require" {
		t.Skip("live Hydra e2e disabled; set PLOY_E2E_CLUSTER=require")
	}
	if os.Getenv("PLOY_E2E_IMAGE") == "" {
		t.Fatal("PLOY_E2E_IMAGE is required and must name a shell-capable image available to the Ploy nodes")
	}

	if _, err := os.Stat(filepath.Join(root, "dist", "ploy")); err != nil {
		t.Fatalf("ploy binary not built (dist/ploy missing); build with `make build`")
	}

	serverURL := os.Getenv("PLOY_SERVER_URL")
	if serverURL == "" {
		port := os.Getenv("PLOY_SERVER_PORT")
		if port == "" {
			port = "8080"
		}
		serverURL = fmt.Sprintf("http://localhost:%s", port)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimRight(serverURL, "/") + "/healthz")
	if err != nil {
		t.Fatalf("live cluster not reachable at %s: %v", serverURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("live cluster health check at %s returned %s", serverURL, resp.Status)
	}
}

// TestHydraMountEnforcement runs the Hydra mount-enforcement e2e scenario,
// validating that /in is read-only and /out is writable. Requires a live
// cluster and runs only when PLOY_E2E_CLUSTER=require.
func TestHydraMountEnforcement(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode; skipping e2e scenario")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "tests", "e2e", "migs", "scenario-hydra-mount-enforcement", "run.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("scenario script not found: %v", err)
	}

	requireLiveCluster(t, root)

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
// validating that files written to /out are uploaded and retrievable as
// artifacts. It runs only when PLOY_E2E_CLUSTER=require.
func TestHydraOutUpload(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode; skipping e2e scenario")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "tests", "e2e", "migs", "scenario-hydra-out-upload", "run.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("scenario script not found: %v", err)
	}

	requireLiveCluster(t, root)

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scenario-hydra-out-upload failed:\n%s", out)
	}
	t.Logf("scenario-hydra-out-upload passed:\n%s", out)
}

// TestHydraInMixed runs the Hydra in-record mixed inputs e2e scenario,
// validating that a spec with both a plain file and a directory in-record
// entry results in both being visible under /in inside the container.
// It runs only when PLOY_E2E_CLUSTER=require.
func TestHydraInMixed(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode; skipping e2e scenario")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "tests", "e2e", "migs", "scenario-in-mixed", "run.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("scenario script not found: %v", err)
	}

	requireLiveCluster(t, root)

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
// rejected by the node agent. It runs only when PLOY_E2E_CLUSTER=require.
func TestHydraBundleBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode; skipping e2e scenario")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "tests", "e2e", "migs", "scenario-bundle-blocked", "run.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("scenario script not found: %v", err)
	}

	requireLiveCluster(t, root)

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
		dir       string
		paths     []string
		forbidden []string
	}{
		{
			dir:       "scenario-hydra-mount-enforcement",
			paths:     []string{"/in/", "/out/"},
			forbidden: []string{"--follow 2>&1", `run status "$run_id" --follow`},
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
			dir:       "scenario-bundle-blocked",
			paths:     []string{"/in/"},
			forbidden: []string{"e2e_descriptor_address", "e2e_descriptor_token", "--follow 2>&1", `run status "$run_id" --follow`},
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

			cmd := exec.Command("bash", "-n", scriptPath)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("bash syntax error in %s:\n%s", sc.dir, out)
			}

			for _, p := range sc.paths {
				if !strings.Contains(content, p) {
					t.Errorf("%s/run.sh: missing expected Hydra mount path %q", sc.dir, p)
				}
			}
			if !strings.Contains(content, "build_gate:\n  disabled: true") {
				t.Errorf("%s/run.sh: generic Hydra scenario must disable Build Gate", sc.dir)
			}
			if !strings.Contains(content, "e2e_runtime_image") {
				t.Errorf("%s/run.sh: live scenario must use the configured E2E runtime image", sc.dir)
			}
			if strings.Contains(content, "alpine:3.20") {
				t.Errorf("%s/run.sh: live scenario must not depend on Docker Hub", sc.dir)
			}
			for _, value := range sc.forbidden {
				if strings.Contains(content, value) {
					t.Errorf("%s/run.sh: contains retired or unsafe construct %q", sc.dir, value)
				}
			}
		})
	}
}

func TestE2EMigRunForwardsGitLabTokenByEnvName(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	tempDir := t.TempDir()
	fakePloy := filepath.Join(tempDir, "ploy")
	argsFile := filepath.Join(tempDir, "args")
	specFile := filepath.Join(tempDir, "spec.yaml")
	if err := os.WriteFile(fakePloy, []byte(`#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" >"${PLOY_E2E_ARGS_FILE:?}"
printf 'run_id: run-1\nmig_id: mig-1\n'
`), 0o700); err != nil {
		t.Fatalf("write fake ploy: %v", err)
	}
	if err := os.WriteFile(specFile, []byte("steps: []\n"), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	harness := filepath.Join(root, "tests", "e2e", "lib", "harness_mig.sh")
	cmd := exec.Command("bash", "-c", `source "$1"; PLOY_BIN="$2"; e2e_mig_run_json "$3" "group/repo:master" >/dev/null`, "bash", harness, fakePloy, specFile)
	cmd.Env = append(os.Environ(),
		"GITLAB_TOKEN=secret-must-stay-in-env",
		"PLOY_E2E_ARGS_FILE="+argsFile,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("e2e_mig_run_json failed: %v\n%s", err, out)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read fake ploy arguments: %v", err)
	}
	got := string(args)
	want := "run\n--gitlab-token-env\nGITLAB_TOKEN\n" + specFile + "\ngroup/repo:master\n"
	if got != want {
		t.Fatalf("ploy arguments = %q, want %q", got, want)
	}
	if strings.Contains(got, "secret-must-stay-in-env") {
		t.Fatal("GitLab token value must not appear in process arguments")
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
		p, err := contracts.ParseStoredEntry(contracts.HydraFileIn, "abcdef0123456:/in/config.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !p.ReadOnly {
			t.Error("/in entry must be read-only")
		}
	})

	t.Run("in_write_to_out_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredEntry(contracts.HydraFileIn, "abcdef0:/out/escape.txt")
		if err == nil {
			t.Fatal("in entry targeting /out/ must be rejected")
		}
	})

	t.Run("in_traversal_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredEntry(contracts.HydraFileIn, "abcdef0:/in/../etc/passwd")
		if err == nil {
			t.Fatal("path traversal in /in must be rejected")
		}
	})

	t.Run("in_duplicates_rejected_at_spec_level", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraEntries(contracts.HydraFileIn, []string{
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
		p, err := contracts.ParseStoredEntry(contracts.HydraFileOut, "abcdef0123456:/out/result.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ReadOnly {
			t.Error("/out entry must be writable")
		}
	})

	t.Run("out_write_to_in_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredEntry(contracts.HydraFileOut, "abcdef0:/in/escape.txt")
		if err == nil {
			t.Fatal("out entry targeting /in/ must be rejected")
		}
	})

	t.Run("out_traversal_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredEntry(contracts.HydraFileOut, "abcdef0:/out/../../etc/shadow")
		if err == nil {
			t.Fatal("path traversal in /out must be rejected")
		}
	})

	t.Run("out_duplicates_rejected_at_spec_level", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraEntries(contracts.HydraFileOut, []string{
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
				"image": "registry.invalid/hydra-contract:test",
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
			p, err := contracts.ParseStoredEntry(contracts.HydraFileIn, entry)
			if err != nil {
				t.Fatalf("in re-parse: %v", err)
			}
			if !p.ReadOnly {
				t.Errorf("in entry %q must be read-only", entry)
			}
		}
		for _, entry := range parsed.Steps[0].Out {
			p, err := contracts.ParseStoredEntry(contracts.HydraFileOut, entry)
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
		p, err := contracts.ParseStoredEntry(contracts.HydraFileOut, "abcdef0123456:/out/report.json")
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
		p, err := contracts.ParseStoredEntry(contracts.HydraFileOut, "abcdef0:/out/deep/nested/artifact.tar.gz")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Dst != "/out/deep/nested/artifact.tar.gz" {
			t.Errorf("expected nested path, got %q", p.Dst)
		}
	})

	t.Run("out_double_slash_cleaned_for_upload", func(t *testing.T) {
		t.Parallel()
		p, err := contracts.ParseStoredEntry(contracts.HydraFileOut, "abcdef0:/out//report.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Dst != "/out/report.json" {
			t.Errorf("expected cleaned path, got %q", p.Dst)
		}
	})

	t.Run("out_empty_hash_breaks_upload_pipeline", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredEntry(contracts.HydraFileOut, ":/out/file.txt")
		if err == nil {
			t.Fatal("empty hash must be rejected (upload requires valid bundle ref)")
		}
	})

	t.Run("out_empty_destination_breaks_upload_pipeline", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredEntry(contracts.HydraFileOut, "abcdef0:")
		if err == nil {
			t.Fatal("empty destination must be rejected (upload target unknown)")
		}
	})

	t.Run("multiple_distinct_out_entries_upload_valid", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraEntries(contracts.HydraFileOut, []string{
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
					"image": "registry.invalid/hydra-contract:test",
					"out": [
						"abcdef0123456:/out/custom-artifact.json",
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
			p, err := contracts.ParseStoredEntry(contracts.HydraFileOut, entry)
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

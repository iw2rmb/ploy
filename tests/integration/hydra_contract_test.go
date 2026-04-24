package integration

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestHydraContract_PrecedenceAndEdgeCases validates Hydra spec parsing
// edge cases: field precedence, parser boundary inputs, and mount enforcement.
func TestHydraContract_PrecedenceAndEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("spec_with_envs_ca_in_out_home_accepted", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{
				"image": "alpine:3.20",
				"envs": {"K1": "v1", "K2": "v2"},
				"ca":   ["abcdef0123456"],
				"in":   ["abcdef0123456:/in/config.json"],
				"out":  ["abcdef0123456:/out/result.txt"],
				"home": ["abcdef0123456:.config/app.toml:ro"]
			}],
			"bundle_map": {"abcdef0123456": "bun-123"}
		}`
		parsed, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err != nil {
			t.Fatalf("expected valid spec, got error: %v", err)
		}
		if len(parsed.Steps) != 1 {
			t.Fatalf("expected 1 step, got %d", len(parsed.Steps))
		}
		if len(parsed.Steps[0].CA) != 1 {
			t.Errorf("expected 1 CA entry, got %d", len(parsed.Steps[0].CA))
		}
		if len(parsed.Steps[0].In) != 1 {
			t.Errorf("expected 1 In entry, got %d", len(parsed.Steps[0].In))
		}
		if len(parsed.Steps[0].Out) != 1 {
			t.Errorf("expected 1 Out entry, got %d", len(parsed.Steps[0].Out))
		}
		if len(parsed.Steps[0].Home) != 1 {
			t.Errorf("expected 1 Home entry, got %d", len(parsed.Steps[0].Home))
		}
	})

	t.Run("in_path_traversal_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredInEntry("abcdef0:/in/../etc/passwd")
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
	})

	t.Run("out_path_traversal_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredOutEntry("abcdef0:/out/../../etc/shadow")
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
	})

	t.Run("home_absolute_path_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredHomeEntry("abcdef0:/etc/config")
		if err == nil {
			t.Fatal("expected error for absolute home path")
		}
	})

	t.Run("home_traversal_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredHomeEntry("abcdef0:../../etc/passwd")
		if err == nil {
			t.Fatal("expected error for home traversal")
		}
	})

	t.Run("duplicate_in_destinations_rejected", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraInEntries([]string{
			"abcdef0:/in/config.json",
			"bbbbbbb:/in/config.json",
		}, "test")
		if err == nil {
			t.Fatal("expected duplicate destination error")
		}
		if !strings.Contains(err.Error(), "duplicate destination") {
			t.Fatalf("error %q does not mention duplicate destination", err.Error())
		}
	})

	t.Run("duplicate_ca_hashes_rejected", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraCAEntries([]string{"abcdef0", "abcdef0"}, "test")
		if err == nil {
			t.Fatal("expected duplicate hash error")
		}
	})

	t.Run("envs_key_precedence_spec_wins_over_empty", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{
				"image": "alpine:3.20",
				"envs": {"K1": "from-spec", "K2": "from-spec"}
			}]
		}`
		parsed, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.Steps[0].Envs["K1"] != "from-spec" {
			t.Errorf("expected K1=from-spec, got %q", parsed.Steps[0].Envs["K1"])
		}
	})

	t.Run("hash_boundary_7_chars_valid", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredCAEntry("abcdef0")
		if err != nil {
			t.Fatalf("7-char hash should be valid: %v", err)
		}
	})

	t.Run("hash_boundary_6_chars_invalid", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredCAEntry("abcdef")
		if err == nil {
			t.Fatal("6-char hash should be invalid")
		}
	})

	t.Run("hash_boundary_64_chars_valid", func(t *testing.T) {
		t.Parallel()
		hash := strings.Repeat("a", 64)
		_, err := contracts.ParseStoredCAEntry(hash)
		if err != nil {
			t.Fatalf("64-char hash should be valid: %v", err)
		}
	})

	t.Run("hash_boundary_65_chars_invalid", func(t *testing.T) {
		t.Parallel()
		hash := strings.Repeat("a", 65)
		_, err := contracts.ParseStoredCAEntry(hash)
		if err == nil {
			t.Fatal("65-char hash should be invalid")
		}
	})

	t.Run("home_ro_suffix_parsed", func(t *testing.T) {
		t.Parallel()
		parsed, err := contracts.ParseStoredHomeEntry("abcdef0:.config/app.toml:ro")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !parsed.ReadOnly {
			t.Error("expected read-only flag")
		}
		if parsed.Dst != ".config/app.toml" {
			t.Errorf("expected .config/app.toml, got %q", parsed.Dst)
		}
	})

	t.Run("home_default_rw", func(t *testing.T) {
		t.Parallel()
		parsed, err := contracts.ParseStoredHomeEntry("abcdef0:.config/app.toml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.ReadOnly {
			t.Error("expected read-write by default")
		}
	})

	t.Run("in_double_slash_cleaned", func(t *testing.T) {
		t.Parallel()
		parsed, err := contracts.ParseStoredInEntry("abcdef0:/in//subdir/file.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.Dst != "/in/subdir/file.txt" {
			t.Errorf("expected cleaned path, got %q", parsed.Dst)
		}
	})

}

// TestHydraContract_MountEnforcement validates that Hydra contract-level
// mount enforcement holds: /in entries are always read-only, /out entries
// are always writable, and violations are rejected at parse time.
func TestHydraContract_MountEnforcement(t *testing.T) {
	t.Parallel()

	t.Run("in_entries_always_readonly", func(t *testing.T) {
		t.Parallel()
		parsed, err := contracts.ParseStoredInEntry("abcdef0:/in/config.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !parsed.ReadOnly {
			t.Error("/in entry must be read-only")
		}
	})

	t.Run("out_entries_always_writable", func(t *testing.T) {
		t.Parallel()
		parsed, err := contracts.ParseStoredOutEntry("abcdef0:/out/result.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.ReadOnly {
			t.Error("/out entry must be writable")
		}
	})

	t.Run("in_rejects_out_prefix", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredInEntry("abcdef0:/out/escape.txt")
		if err == nil {
			t.Fatal("in entry with /out/ prefix should be rejected")
		}
	})

	t.Run("out_rejects_in_prefix", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredOutEntry("abcdef0:/in/escape.txt")
		if err == nil {
			t.Fatal("out entry with /in/ prefix should be rejected")
		}
	})

	t.Run("in_rejects_root_path", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredInEntry("abcdef0:/etc/passwd")
		if err == nil {
			t.Fatal("in entry outside /in/ should be rejected")
		}
	})

	t.Run("out_rejects_root_path", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredOutEntry("abcdef0:/etc/shadow")
		if err == nil {
			t.Fatal("out entry outside /out/ should be rejected")
		}
	})

	t.Run("spec_level_in_readonly_out_writable", func(t *testing.T) {
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
		// Validate via re-parsing individual entries from the spec.
		for _, entry := range parsed.Steps[0].In {
			p, err := contracts.ParseStoredInEntry(entry)
			if err != nil {
				t.Fatalf("in re-parse: %v", err)
			}
			if !p.ReadOnly {
				t.Errorf("in entry %q should be read-only", entry)
			}
		}
		for _, entry := range parsed.Steps[0].Out {
			p, err := contracts.ParseStoredOutEntry(entry)
			if err != nil {
				t.Fatalf("out re-parse: %v", err)
			}
			if p.ReadOnly {
				t.Errorf("out entry %q should be writable", entry)
			}
		}
	})

	t.Run("home_traversal_above_home_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredHomeEntry("abcdef0:../../../etc/passwd")
		if err == nil {
			t.Fatal("home entry traversing above $HOME should be rejected")
		}
	})

	t.Run("duplicate_out_destinations_rejected", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraOutEntries([]string{
			"abcdef0:/out/result.json",
			"bbbbbbb:/out/result.json",
		}, "test")
		if err == nil {
			t.Fatal("expected duplicate destination error for out entries")
		}
		if !strings.Contains(err.Error(), "duplicate destination") {
			t.Fatalf("error %q does not mention duplicate destination", err.Error())
		}
	})

	t.Run("duplicate_home_destinations_rejected", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraHomeEntries([]string{
			"abcdef0:.config/app.toml",
			"bbbbbbb:.config/app.toml",
		}, "test")
		if err == nil {
			t.Fatal("expected duplicate destination error for home entries")
		}
		if !strings.Contains(err.Error(), "duplicate destination") {
			t.Fatalf("error %q does not mention duplicate destination", err.Error())
		}
	})
}

// TestHydraContract_OutUploadContinuity validates that out entries maintain
// structural integrity required for upload continuity: valid hashes, proper
// /out/ prefix, distinct destinations, and correct writable semantics.
func TestHydraContract_OutUploadContinuity(t *testing.T) {
	t.Parallel()

	t.Run("valid_out_entry_has_hash_and_destination", func(t *testing.T) {
		t.Parallel()
		parsed, err := contracts.ParseStoredOutEntry("abcdef0123456:/out/artifact.tar.gz")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.Hash != "abcdef0123456" {
			t.Errorf("expected hash abcdef0123456, got %q", parsed.Hash)
		}
		if parsed.Dst != "/out/artifact.tar.gz" {
			t.Errorf("expected dst /out/artifact.tar.gz, got %q", parsed.Dst)
		}
	})

	t.Run("out_nested_subdirectory_accepted", func(t *testing.T) {
		t.Parallel()
		parsed, err := contracts.ParseStoredOutEntry("abcdef0:/out/deep/nested/file.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.Dst != "/out/deep/nested/file.txt" {
			t.Errorf("expected /out/deep/nested/file.txt, got %q", parsed.Dst)
		}
	})

	t.Run("out_double_slash_cleaned", func(t *testing.T) {
		t.Parallel()
		parsed, err := contracts.ParseStoredOutEntry("abcdef0:/out//artifact.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.Dst != "/out/artifact.txt" {
			t.Errorf("expected cleaned path /out/artifact.txt, got %q", parsed.Dst)
		}
	})

	t.Run("out_traversal_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredOutEntry("abcdef0:/out/../in/escape.txt")
		if err == nil {
			t.Fatal("out traversal into /in should be rejected")
		}
	})

	t.Run("out_empty_hash_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredOutEntry(":/out/file.txt")
		if err == nil {
			t.Fatal("out entry with empty hash should be rejected")
		}
	})

	t.Run("out_empty_destination_rejected", func(t *testing.T) {
		t.Parallel()
		_, err := contracts.ParseStoredOutEntry("abcdef0:")
		if err == nil {
			t.Fatal("out entry with empty destination should be rejected")
		}
	})

	t.Run("multiple_distinct_out_entries_accepted", func(t *testing.T) {
		t.Parallel()
		err := contracts.ValidateHydraOutEntries([]string{
			"abcdef0:/out/result-a.json",
			"bbbbbbb:/out/result-b.json",
			"ccccccc:/out/nested/result-c.txt",
		}, "test")
		if err != nil {
			t.Fatalf("expected distinct out entries to be valid: %v", err)
		}
	})

	t.Run("spec_with_out_entries_roundtrip", func(t *testing.T) {
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
		// Verify each entry retains its hash and destination for upload.
		for _, entry := range parsed.Steps[0].Out {
			p, err := contracts.ParseStoredOutEntry(entry)
			if err != nil {
				t.Fatalf("re-parse out entry: %v", err)
			}
			if p.Hash == "" {
				t.Error("out entry hash must not be empty")
			}
			if !strings.HasPrefix(p.Dst, "/out/") {
				t.Errorf("out entry destination must start with /out/, got %q", p.Dst)
			}
		}
	})
}

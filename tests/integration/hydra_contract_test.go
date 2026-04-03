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

	t.Run("legacy_env_from_file_rejected", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{"image": "alpine:3.20", "env_from_file": {"K": "v"}}]
		}`
		_, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err == nil {
			t.Fatal("expected error for env_from_file, got nil")
		}
		if !strings.Contains(err.Error(), "env_from_file") {
			t.Fatalf("error %q does not mention env_from_file", err.Error())
		}
	})

	t.Run("legacy_tmp_bundle_rejected", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{"image": "alpine:3.20", "tmp_bundle": {}}]
		}`
		_, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err == nil {
			t.Fatal("expected error for tmp_bundle, got nil")
		}
		if !strings.Contains(err.Error(), "tmp_bundle") {
			t.Fatalf("error %q does not mention tmp_bundle", err.Error())
		}
	})

	t.Run("legacy_tmp_dir_rejected", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{"image": "alpine:3.20", "tmp_dir": []}]
		}`
		_, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err == nil {
			t.Fatal("expected error for tmp_dir, got nil")
		}
		if !strings.Contains(err.Error(), "tmp_dir") {
			t.Fatalf("error %q does not mention tmp_dir", err.Error())
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

	t.Run("router_with_hydra_fields_accepted", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{"image": "alpine:3.20"}],
			"build_gate": {
				"enabled": true,
				"router": {
					"image": "codex:latest",
					"envs": {"CODEX_PROMPT": "hello"},
					"in": ["abcdef0:/in/auth.json"],
					"ca": ["bbbbbbb0123456"]
				}
			},
			"bundle_map": {"abcdef0": "bun-1", "bbbbbbb0123456": "bun-2"}
		}`
		parsed, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err != nil {
			t.Fatalf("expected valid spec, got error: %v", err)
		}
		if parsed.BuildGate == nil || parsed.BuildGate.Router == nil {
			t.Fatal("expected router to be parsed")
		}
	})

	t.Run("healing_action_with_hydra_fields_accepted", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{"image": "alpine:3.20"}],
			"build_gate": {
				"router": {
					"image": "codex:latest",
					"envs": {"CODEX_PROMPT": "route it"}
				},
				"healing": {
					"by_error_kind": {
						"code": {
							"retries": 1,
							"image": "codex:latest",
							"envs": {"CODEX_PROMPT": "fix it"},
							"in": ["abcdef0:/in/auth.json"]
						}
					}
				}
			},
			"bundle_map": {"abcdef0": "bun-1"}
		}`
		parsed, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err != nil {
			t.Fatalf("expected valid spec, got error: %v", err)
		}
		if parsed.BuildGate == nil || parsed.BuildGate.Healing == nil || len(parsed.BuildGate.Healing.ByErrorKind) == 0 {
			t.Fatal("expected healing action to be parsed")
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

	t.Run("legacy_env_rejected_at_root", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"env": {"KEY": "val"},
			"steps": [{"image": "alpine:3.20"}]
		}`
		_, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err == nil {
			t.Fatal("expected error for root-level env (legacy)")
		}
	})

	t.Run("legacy_env_from_file_rejected_in_router", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{"image": "alpine:3.20"}],
			"build_gate": {
				"router": {
					"image": "codex:latest",
					"env_from_file": {"K": "path"}
				}
			}
		}`
		_, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err == nil {
			t.Fatal("expected error for env_from_file in router")
		}
		if !strings.Contains(err.Error(), "env_from_file") {
			t.Fatalf("error %q does not mention env_from_file", err.Error())
		}
	})

	t.Run("legacy_env_from_file_rejected_in_healing_action", func(t *testing.T) {
		t.Parallel()
		spec := `{
			"steps": [{"image": "alpine:3.20"}],
			"build_gate": {
				"healing": {
					"by_error_kind": {
						"code": {
							"image": "codex:latest",
							"env_from_file": {"K": "path"}
						}
					}
				}
			}
		}`
		_, err := contracts.ParseMigSpecJSON([]byte(spec))
		if err == nil {
			t.Fatal("expected error for env_from_file in healing action")
		}
		if !strings.Contains(err.Error(), "env_from_file") {
			t.Fatalf("error %q does not mention env_from_file", err.Error())
		}
	})
}

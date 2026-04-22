package migs_e2e_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/handlers"
)

// TestHydraContractOffline validates the Hydra migration and mount contract
// without requiring a built binary or running cluster. This test MUST NOT
// skip — it is the offline proof that Hydra e2e flows are correct on a clean
// workspace.
func TestHydraContractOffline(t *testing.T) {
	table := handlers.SpecialEnvMappingTable()

	t.Run("mapping_table_non_empty", func(t *testing.T) {
		if len(table) == 0 {
			t.Fatal("SpecialEnvMappingTable is empty")
		}
	})

	t.Run("mapping_table_sorted", func(t *testing.T) {
		keys := make([]string, len(table))
		for i, m := range table {
			keys[i] = m.EnvKey
		}
		if !sort.StringsAreSorted(keys) {
			t.Errorf("mapping table not sorted: %v", keys)
		}
	})

	t.Run("mapping_table_no_duplicates", func(t *testing.T) {
		seen := map[string]bool{}
		for _, m := range table {
			if seen[m.EnvKey] {
				t.Errorf("duplicate mapping for key %q", m.EnvKey)
			}
			seen[m.EnvKey] = true
		}
	})

	t.Run("all_fields_valid", func(t *testing.T) {
		validFields := map[string]bool{"ca": true, "home": true, "in": true}
		for _, m := range table {
			if !validFields[m.TargetField] {
				t.Errorf("key %q has invalid TargetField %q", m.EnvKey, m.TargetField)
			}
		}
	})

	t.Run("home_entries_have_destination", func(t *testing.T) {
		for _, m := range table {
			if m.TargetField == "home" && m.Destination == "" {
				t.Errorf("key %q: home field requires non-empty Destination", m.EnvKey)
			}
		}
	})

	t.Run("in_entries_have_destination", func(t *testing.T) {
		for _, m := range table {
			if m.TargetField == "in" && m.Destination == "" {
				t.Errorf("key %q: in field requires non-empty Destination", m.EnvKey)
			}
		}
	})

	t.Run("rewrite_round_trip", func(t *testing.T) {
		for _, m := range table {
			mapping := handlers.LookupSpecialEnvMapping(m.EnvKey)
			if mapping == nil {
				t.Fatalf("LookupSpecialEnvMapping(%q) returned nil", m.EnvKey)
			}
			field, entry := handlers.RewriteSpecialEnvEntry(mapping, "testhash1234")
			if field != m.TargetField {
				t.Errorf("key %q: RewriteSpecialEnvEntry field = %q, want %q", m.EnvKey, field, m.TargetField)
			}
			if entry == "" {
				t.Errorf("key %q: RewriteSpecialEnvEntry produced empty entry", m.EnvKey)
			}
			// Verify entry contains the hash.
			if !strings.Contains(entry, "testhash1234") {
				t.Errorf("key %q: entry %q does not contain hash", m.EnvKey, entry)
			}
			// For home/in, verify entry contains the destination.
			if m.TargetField == "home" || m.TargetField == "in" {
				if !strings.Contains(entry, m.Destination) {
					t.Errorf("key %q: entry %q does not contain destination %q", m.EnvKey, entry, m.Destination)
				}
			}
		}
	})

	// Cross-check: docs inventories must list every special env key or its
	// typed target field (legacy keys migrated to typed fields like "ca" are
	// documented under the target field name, not the original env key).
	t.Run("docs_env_readme_lists_all_keys", func(t *testing.T) {
		root := repoRoot(t)
		data, err := os.ReadFile(filepath.Join(root, "docs", "envs", "README.md"))
		if err != nil {
			t.Fatalf("read docs/envs/README.md: %v", err)
		}
		content := string(data)
		for _, m := range table {
			if !strings.Contains(content, m.EnvKey) && !strings.Contains(content, m.TargetField) {
				t.Errorf("docs/envs/README.md does not mention migrated key %q or its target field %q", m.EnvKey, m.TargetField)
			}
		}
	})

	t.Run("docs_api_config_env_documents_reserved_key_rejection", func(t *testing.T) {
		root := repoRoot(t)
		data, err := os.ReadFile(filepath.Join(root, "docs", "api", "paths", "config_env_key.yaml"))
		if err != nil {
			t.Fatalf("read docs/api/paths/config_env_key.yaml: %v", err)
		}
		content := string(data)

		// The API doc must describe reserved-key rejection for each Hydra
		// typed field that has special env mappings.
		fields := map[string]bool{}
		for _, m := range table {
			fields[m.TargetField] = true
		}
		for field := range fields {
			if !strings.Contains(content, field) {
				t.Errorf("docs/api/paths/config_env_key.yaml does not mention typed field %q in reserved-key description", field)
			}
		}
		if !strings.Contains(content, "eserved") {
			t.Error("docs/api/paths/config_env_key.yaml does not document reserved-key rejection")
		}
	})

}

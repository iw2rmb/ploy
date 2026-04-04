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

	// Cross-check: docs inventories must list every special env key.
	t.Run("docs_env_readme_lists_all_keys", func(t *testing.T) {
		root := repoRoot(t)
		data, err := os.ReadFile(filepath.Join(root, "docs", "envs", "README.md"))
		if err != nil {
			t.Fatalf("read docs/envs/README.md: %v", err)
		}
		content := string(data)
		for _, m := range table {
			if !strings.Contains(content, m.EnvKey) {
				t.Errorf("docs/envs/README.md does not mention migrated key %q", m.EnvKey)
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

	// Cross-check: scenario scripts must reference Hydra mount paths, not legacy env keys.
	t.Run("scenarios_use_hydra_not_legacy_env", func(t *testing.T) {
		root := repoRoot(t)
		scenarioDir := filepath.Join(root, "tests", "e2e", "migs")
		entries, err := os.ReadDir(scenarioDir)
		if err != nil {
			t.Fatalf("read scenario dir: %v", err)
		}

		scenarioCount := 0
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "scenario-hydra-") {
				continue
			}
			scenarioCount++
			scriptPath := filepath.Join(scenarioDir, e.Name(), "run.sh")
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatalf("%s/run.sh missing: %v", e.Name(), err)
			}
			content := string(data)

			// Must reference at least one Hydra mount path.
			hasHydraPath := strings.Contains(content, "/in/") || strings.Contains(content, "/out/")
			if !hasHydraPath {
				t.Errorf("%s/run.sh: no Hydra mount paths (/in/ or /out/) found", e.Name())
			}

			// Must not inject legacy special env keys as env vars.
			for _, m := range table {
				// Skip checking for keys that appear in comments or assertions.
				// Only flag direct env injection patterns.
				if strings.Contains(content, m.EnvKey+"=") {
					t.Errorf("%s/run.sh: contains legacy env injection %s=; use Hydra mount", e.Name(), m.EnvKey)
				}
			}
		}

		if scenarioCount == 0 {
			t.Fatal("no scenario-hydra-* directories found; expected at least 2")
		}
	})
}

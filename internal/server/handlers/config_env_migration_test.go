package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestSpecialEnvMappingTable_DesignAlignment(t *testing.T) {
	// Verify each mapping matches the design doc's special env migration table.
	tests := []struct {
		key   string
		field string
		dst   string
		mode  string
	}{
		{"CODEX_AUTH_JSON", "home", ".codex/auth.json", "ro"},
		{"CODEX_CONFIG_TOML", "home", ".codex/config.toml", "ro"},
		{"CRUSH_JSON", "home", ".config/crush/crush.json", "ro"},
		{"CCR_CONFIG_JSON", "home", ".claude-code/config.json", "ro"},
	}

	table := SpecialEnvMappingTable()
	if len(table) != len(tests) {
		t.Fatalf("mapping table has %d entries, want %d", len(table), len(tests))
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m := LookupSpecialEnvMapping(tt.key)
			if m == nil {
				t.Fatalf("no mapping for key %q", tt.key)
			}
			if m.TargetField != tt.field {
				t.Errorf("TargetField = %q, want %q", m.TargetField, tt.field)
			}
			if m.Destination != tt.dst {
				t.Errorf("Destination = %q, want %q", m.Destination, tt.dst)
			}
			if m.Mode != tt.mode {
				t.Errorf("Mode = %q, want %q", m.Mode, tt.mode)
			}
		})
	}
}

func TestIsSpecialEnvKey(t *testing.T) {
	special := []string{
		"CODEX_AUTH_JSON", "CODEX_CONFIG_TOML",
		"CRUSH_JSON", "CCR_CONFIG_JSON",
	}
	for _, k := range special {
		if !IsSpecialEnvKey(k) {
			t.Errorf("IsSpecialEnvKey(%q) = false, want true", k)
		}
	}

	notSpecial := []string{"OPENAI_API_KEY", "PLOY_GRADLE_BUILD_CACHE_URL", "PLOY_CA_CERTS", "PATH", ""}
	for _, k := range notSpecial {
		if IsSpecialEnvKey(k) {
			t.Errorf("IsSpecialEnvKey(%q) = true, want false", k)
		}
	}
}

func TestRewriteSpecialEnvEntry(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		hash      string
		wantField string
		wantEntry string
	}{
		{"home_auth", "CODEX_AUTH_JSON", "def5678", "home", "def5678:.codex/auth.json:ro"},
		{"home_config", "CODEX_CONFIG_TOML", "aaa1111", "home", "aaa1111:.codex/config.toml:ro"},
		{"home_crush", "CRUSH_JSON", "bbb2222", "home", "bbb2222:.config/crush/crush.json:ro"},
		{"home_ccr", "CCR_CONFIG_JSON", "ccc3333", "home", "ccc3333:.claude-code/config.json:ro"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := LookupSpecialEnvMapping(tt.key)
			if m == nil {
				t.Fatalf("no mapping for key %q", tt.key)
			}
			field, entry := RewriteSpecialEnvEntry(m, tt.hash)
			if field != tt.wantField {
				t.Errorf("field = %q, want %q", field, tt.wantField)
			}
			if entry != tt.wantEntry {
				t.Errorf("entry = %q, want %q", entry, tt.wantEntry)
			}
		})
	}
}

func TestScanSpecialEnvKeys_RewriteCandidates(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"CODEX_CONFIG_TOML": {{Value: "toml-data", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
		"CODEX_AUTH_JSON":   {{Value: `{"token":"xxx"}`, Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
		"OPENAI_API_KEY":    {{Value: "sk-xxx", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, nil, nil)

	if report.Rewritten != 2 {
		t.Errorf("Rewritten = %d, want 2", report.Rewritten)
	}
	if report.Rejected != 0 {
		t.Errorf("Rejected = %d, want 0", report.Rejected)
	}
	if report.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", report.Skipped)
	}
	if len(report.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2", len(report.Entries))
	}

	// Entries are sorted by key name.
	if report.Entries[0].EnvKey != "CODEX_AUTH_JSON" {
		t.Errorf("Entries[0].EnvKey = %q, want CODEX_AUTH_JSON", report.Entries[0].EnvKey)
	}
	if report.Entries[1].EnvKey != "CODEX_CONFIG_TOML" {
		t.Errorf("Entries[1].EnvKey = %q, want CODEX_CONFIG_TOML", report.Entries[1].EnvKey)
	}
}

// TestScanSpecialEnvKeys_TargetMapping verifies that each GlobalEnvTarget maps
// to the correct set of job sections.
func TestScanSpecialEnvKeys_TargetMapping(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		target       domaintypes.GlobalEnvTarget
		wantSections []string
	}{
		{"gates", "CRUSH_JSON", domaintypes.GlobalEnvTargetGates, []string{"pre_gate", "re_gate", "post_gate"}},
		{"steps", "CODEX_CONFIG_TOML", domaintypes.GlobalEnvTargetSteps, []string{"heal", "mig"}},
		{"server expands to all", "CRUSH_JSON", domaintypes.GlobalEnvTargetServer, nil},  // 5 sections
		{"nodes expands to all", "CRUSH_JSON", domaintypes.GlobalEnvTargetNodes, nil},    // 5 sections
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalEnv := map[string][]GlobalEnvVar{
				tt.key: {{Value: "data", Target: tt.target, Secret: true}},
			}

			report := ScanSpecialEnvKeys(globalEnv, nil, nil, nil)

			if report.Rewritten != 1 {
				t.Fatalf("Rewritten = %d, want 1", report.Rewritten)
			}
			if report.Entries[0].Action != MigrationActionRewrite {
				t.Errorf("Action = %q, want %q", report.Entries[0].Action, MigrationActionRewrite)
			}

			if tt.wantSections != nil {
				if len(report.Entries[0].Sections) != len(tt.wantSections) {
					t.Fatalf("Sections = %v, want %v", report.Entries[0].Sections, tt.wantSections)
				}
				for i, s := range tt.wantSections {
					if report.Entries[0].Sections[i] != s {
						t.Errorf("Sections[%d] = %q, want %q", i, report.Entries[0].Sections[i], s)
					}
				}
			} else {
				// Server/nodes target maps to all 5 job sections.
				if len(report.Entries[0].Sections) != 5 {
					t.Errorf("Sections = %v, want 5 sections (all)", report.Entries[0].Sections)
				}
			}
		})
	}
}

// TestScanSpecialEnvKeys_ConflictCases verifies that existing home/in entries
// cause rejection when they conflict with a special env migration.
func TestScanSpecialEnvKeys_ConflictCases(t *testing.T) {
	tests := []struct {
		name         string
		globalEnv    map[string][]GlobalEnvVar
		existingHome map[string][]ConfigHomeEntry
	}{
		{
			name: "home conflict full",
			globalEnv: map[string][]GlobalEnvVar{
				"CODEX_AUTH_JSON": {{Value: `{"token":"xxx"}`, Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
			},
			existingHome: map[string][]ConfigHomeEntry{
				"mig": {{Entry: "existinghash:.codex/auth.json:ro", Dst: ".codex/auth.json", Section: "mig"}},
			},
		},
		{
			name: "home conflict partial sections",
			globalEnv: map[string][]GlobalEnvVar{
				"CODEX_AUTH_JSON": {{Value: `{"token":"xxx"}`, Target: domaintypes.GlobalEnvTargetSteps}},
			},
			existingHome: map[string][]ConfigHomeEntry{
				"mig": {{Entry: "abc1234:.codex/auth.json:ro", Dst: ".codex/auth.json", Section: "mig"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := ScanSpecialEnvKeys(tt.globalEnv, nil, tt.existingHome, nil)

			if report.Rejected != 1 {
				t.Fatalf("Rejected = %d, want 1", report.Rejected)
			}
			if report.Entries[0].Action != MigrationActionReject {
				t.Errorf("Action = %q, want %q", report.Entries[0].Action, MigrationActionReject)
			}
			if report.Entries[0].Reason == "" {
				t.Error("Reason should not be empty for rejected entries")
			}
		})
	}
}

func TestScanSpecialEnvKeys_MultipleTargets(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"CRUSH_JSON": {
			{Value: "crush-data", Target: domaintypes.GlobalEnvTargetGates, Secret: true},
			{Value: "crush-data", Target: domaintypes.GlobalEnvTargetSteps, Secret: true},
			{Value: "crush-data", Target: domaintypes.GlobalEnvTargetServer, Secret: true},
		},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, nil, nil)

	if report.Rewritten != 3 {
		t.Errorf("Rewritten = %d, want 3 (gates + steps + server)", report.Rewritten)
	}
	if report.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", report.Skipped)
	}
}

func TestScanSpecialEnvKeys_EmptyInput(t *testing.T) {
	report := ScanSpecialEnvKeys(nil, nil, nil, nil)
	if len(report.Entries) != 0 {
		t.Errorf("Entries = %d, want 0", len(report.Entries))
	}
}

func TestScanSpecialEnvKeys_NonSpecialKeysIgnored(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"OPENAI_API_KEY":              {{Value: "sk-xxx", Target: domaintypes.GlobalEnvTargetSteps}},
		"PLOY_GRADLE_BUILD_CACHE_URL": {{Value: "https://cache.example.com", Target: domaintypes.GlobalEnvTargetGates}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, nil, nil)

	if len(report.Entries) != 0 {
		t.Errorf("Entries = %d, want 0 (non-special keys should be ignored)", len(report.Entries))
	}
}

func TestMigrationReport_Metrics(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"CODEX_AUTH_JSON":   {{Value: "json", Target: domaintypes.GlobalEnvTargetSteps}},
		"CODEX_CONFIG_TOML": {{Value: "toml", Target: domaintypes.GlobalEnvTargetServer}},
		"CRUSH_JSON":        {{Value: "crush", Target: domaintypes.GlobalEnvTargetSteps}},
	}
	existingHome := map[string][]ConfigHomeEntry{
		"mig": {{Entry: "existinghash:.codex/auth.json:ro", Dst: ".codex/auth.json"}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, existingHome, nil)

	// AUTH steps → reject (conflict, hash mismatch),
	// CONFIG server → rewrite (all sections), CRUSH steps → rewrite
	if report.Rewritten != 2 {
		t.Errorf("Rewritten = %d, want 2", report.Rewritten)
	}
	if report.Rejected != 1 {
		t.Errorf("Rejected = %d, want 1", report.Rejected)
	}
	if report.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", report.Skipped)
	}
	total := report.Rewritten + report.Rejected + report.Skipped
	if total != len(report.Entries) {
		t.Errorf("total %d != len(Entries) %d", total, len(report.Entries))
	}
}

func TestLogMigrationReport_NoEntries(t *testing.T) {
	// Smoke test: should not panic on empty report.
	report := &MigrationReport{}
	LogMigrationReport(report)
}

func TestLogMigrationReport_WithEntries(t *testing.T) {
	// Smoke test: should not panic with entries.
	report := &MigrationReport{
		Entries: []MigrationReportEntry{
			{EnvKey: "CRUSH_JSON", Target: "steps", Action: MigrationActionRewrite, TargetField: "home", Sections: []string{"mig"}},
			{EnvKey: "CODEX_AUTH_JSON", Target: "steps", Action: MigrationActionReject, TargetField: "home", Reason: "conflict"},
			{EnvKey: "CRUSH_JSON", Target: "server", Action: MigrationActionSkip, TargetField: "home", Reason: "server target"},
		},
		Rewritten: 1,
		Rejected:  1,
		Skipped:   1,
	}
	LogMigrationReport(report)
}

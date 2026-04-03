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
		{"PLOY_CA_CERTS", "ca", "", ""},
		{"CODEX_AUTH_JSON", "home", ".codex/auth.json", "ro"},
		{"CODEX_CONFIG_TOML", "home", ".codex/config.toml", "ro"},
		{"CRUSH_JSON", "home", ".config/crush/crush.json", "ro"},
		{"CCR_CONFIG_JSON", "home", ".claude-code-router/config.json", "ro"},
		{"CODEX_PROMPT", "in", "/in/codex-prompt.txt", ""},
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
		"PLOY_CA_CERTS", "CODEX_AUTH_JSON", "CODEX_CONFIG_TOML",
		"CRUSH_JSON", "CCR_CONFIG_JSON", "CODEX_PROMPT",
	}
	for _, k := range special {
		if !IsSpecialEnvKey(k) {
			t.Errorf("IsSpecialEnvKey(%q) = false, want true", k)
		}
	}

	notSpecial := []string{"OPENAI_API_KEY", "PLOY_GRADLE_BUILD_CACHE_URL", "PATH", ""}
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
		{"ca", "PLOY_CA_CERTS", "abc1234", "ca", "abc1234"},
		{"home_auth", "CODEX_AUTH_JSON", "def5678", "home", "def5678:.codex/auth.json:ro"},
		{"home_config", "CODEX_CONFIG_TOML", "aaa1111", "home", "aaa1111:.codex/config.toml:ro"},
		{"home_crush", "CRUSH_JSON", "bbb2222", "home", "bbb2222:.config/crush/crush.json:ro"},
		{"home_ccr", "CCR_CONFIG_JSON", "ccc3333", "home", "ccc3333:.claude-code-router/config.json:ro"},
		{"in_prompt", "CODEX_PROMPT", "eee5555", "in", "eee5555:/in/codex-prompt.txt"},
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
		"PLOY_CA_CERTS":  {{Value: "-----BEGIN CERT-----\n...", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
		"CODEX_AUTH_JSON": {{Value: `{"token":"xxx"}`, Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
		"OPENAI_API_KEY":  {{Value: "sk-xxx", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
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
	if report.Entries[1].EnvKey != "PLOY_CA_CERTS" {
		t.Errorf("Entries[1].EnvKey = %q, want PLOY_CA_CERTS", report.Entries[1].EnvKey)
	}
}

func TestScanSpecialEnvKeys_ServerTargetSkipped(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"PLOY_CA_CERTS": {{Value: "cert-data", Target: domaintypes.GlobalEnvTargetServer, Secret: true}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, nil, nil)

	if report.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", report.Skipped)
	}
	if report.Entries[0].Action != MigrationActionSkip {
		t.Errorf("Action = %q, want %q", report.Entries[0].Action, MigrationActionSkip)
	}
}

func TestScanSpecialEnvKeys_NodesTargetSkipped(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"PLOY_CA_CERTS": {{Value: "cert-data", Target: domaintypes.GlobalEnvTargetNodes, Secret: true}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, nil, nil)

	if report.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", report.Skipped)
	}
}

func TestScanSpecialEnvKeys_ConflictRejected(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"CODEX_AUTH_JSON": {{Value: `{"token":"xxx"}`, Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
	}

	existingHome := map[string][]ConfigHomeEntry{
		"mig": {{Entry: "existinghash:.codex/auth.json:ro", Dst: ".codex/auth.json", Section: "mig"}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, existingHome, nil)

	if report.Rejected != 1 {
		t.Fatalf("Rejected = %d, want 1", report.Rejected)
	}
	if report.Entries[0].Action != MigrationActionReject {
		t.Errorf("Action = %q, want %q", report.Entries[0].Action, MigrationActionReject)
	}
	if report.Entries[0].Reason == "" {
		t.Error("Reason should not be empty for rejected entries")
	}
}

func TestScanSpecialEnvKeys_GatesTarget(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"PLOY_CA_CERTS": {{Value: "cert-data", Target: domaintypes.GlobalEnvTargetGates, Secret: true}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, nil, nil)

	if report.Rewritten != 1 {
		t.Fatalf("Rewritten = %d, want 1", report.Rewritten)
	}

	entry := report.Entries[0]
	wantSections := []string{"pre_gate", "re_gate", "post_gate"}
	if len(entry.Sections) != len(wantSections) {
		t.Fatalf("Sections = %v, want %v", entry.Sections, wantSections)
	}
	for i, s := range wantSections {
		if entry.Sections[i] != s {
			t.Errorf("Sections[%d] = %q, want %q", i, entry.Sections[i], s)
		}
	}
}

func TestScanSpecialEnvKeys_StepsTarget(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"CODEX_CONFIG_TOML": {{Value: "toml-data", Target: domaintypes.GlobalEnvTargetSteps}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, nil, nil)

	if report.Rewritten != 1 {
		t.Fatalf("Rewritten = %d, want 1", report.Rewritten)
	}

	entry := report.Entries[0]
	wantSections := []string{"heal", "mig"}
	if len(entry.Sections) != len(wantSections) {
		t.Fatalf("Sections = %v, want %v", entry.Sections, wantSections)
	}
	for i, s := range wantSections {
		if entry.Sections[i] != s {
			t.Errorf("Sections[%d] = %q, want %q", i, entry.Sections[i], s)
		}
	}
}

func TestScanSpecialEnvKeys_MultipleTargets(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"PLOY_CA_CERTS": {
			{Value: "cert-data", Target: domaintypes.GlobalEnvTargetGates, Secret: true},
			{Value: "cert-data", Target: domaintypes.GlobalEnvTargetSteps, Secret: true},
			{Value: "cert-data", Target: domaintypes.GlobalEnvTargetServer, Secret: true},
		},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, nil, nil)

	if report.Rewritten != 2 {
		t.Errorf("Rewritten = %d, want 2 (gates + steps)", report.Rewritten)
	}
	if report.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (server)", report.Skipped)
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

func TestScanSpecialEnvKeys_HomeConflictPartialSections(t *testing.T) {
	// CODEX_AUTH_JSON with steps target maps to [heal, mig].
	// Conflict only in mig → reject (conflict in any section rejects the entry).
	globalEnv := map[string][]GlobalEnvVar{
		"CODEX_AUTH_JSON": {{Value: `{"token":"xxx"}`, Target: domaintypes.GlobalEnvTargetSteps}},
	}

	existingHome := map[string][]ConfigHomeEntry{
		"mig": {{Entry: "abc1234:.codex/auth.json:ro", Dst: ".codex/auth.json", Section: "mig"}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, existingHome, nil)

	if report.Rejected != 1 {
		t.Fatalf("Rejected = %d, want 1", report.Rejected)
	}
}

func TestScanSpecialEnvKeys_InConflictRejected(t *testing.T) {
	// CODEX_PROMPT maps to in:/in/codex-prompt.txt. Existing in entry for that
	// destination in the mig section must cause rejection.
	globalEnv := map[string][]GlobalEnvVar{
		"CODEX_PROMPT": {{Value: "do the thing", Target: domaintypes.GlobalEnvTargetSteps}},
	}

	existingIn := map[string][]ConfigInEntry{
		"mig": {{Entry: "abc123:/in/codex-prompt.txt", Dst: "/in/codex-prompt.txt", Section: "mig"}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, nil, existingIn)

	if report.Rejected != 1 {
		t.Fatalf("Rejected = %d, want 1", report.Rejected)
	}
	if report.Entries[0].Action != MigrationActionReject {
		t.Errorf("Action = %q, want %q", report.Entries[0].Action, MigrationActionReject)
	}
	if report.Entries[0].Reason == "" {
		t.Error("Reason should not be empty for rejected in entries")
	}
}

func TestMigrationReport_Metrics(t *testing.T) {
	globalEnv := map[string][]GlobalEnvVar{
		"PLOY_CA_CERTS":    {{Value: "cert", Target: domaintypes.GlobalEnvTargetGates}},
		"CODEX_AUTH_JSON":   {{Value: "json", Target: domaintypes.GlobalEnvTargetSteps}},
		"CODEX_CONFIG_TOML": {{Value: "toml", Target: domaintypes.GlobalEnvTargetServer}},
		"CRUSH_JSON":        {{Value: "crush", Target: domaintypes.GlobalEnvTargetSteps}},
	}
	existingHome := map[string][]ConfigHomeEntry{
		"mig": {{Dst: ".codex/auth.json"}},
	}

	report := ScanSpecialEnvKeys(globalEnv, nil, existingHome, nil)

	// CA gates → rewrite, AUTH steps → reject (conflict), CONFIG server → skip, CRUSH steps → rewrite
	if report.Rewritten != 2 {
		t.Errorf("Rewritten = %d, want 2", report.Rewritten)
	}
	if report.Rejected != 1 {
		t.Errorf("Rejected = %d, want 1", report.Rejected)
	}
	if report.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", report.Skipped)
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
			{EnvKey: "PLOY_CA_CERTS", Target: "steps", Action: MigrationActionRewrite, TargetField: "ca", Sections: []string{"mig"}},
			{EnvKey: "CODEX_AUTH_JSON", Target: "steps", Action: MigrationActionReject, TargetField: "home", Reason: "conflict"},
			{EnvKey: "CRUSH_JSON", Target: "server", Action: MigrationActionSkip, TargetField: "home", Reason: "server target"},
		},
		Rewritten: 1,
		Rejected:  1,
		Skipped:   1,
	}
	LogMigrationReport(report)
}

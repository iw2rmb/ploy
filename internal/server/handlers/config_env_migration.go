package handlers

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// SpecialEnvMapping defines how a legacy env key maps to a typed Hydra field.
// This is the canonical mapping table from the Hydra design doc (§ Special env
// migration table).
type SpecialEnvMapping struct {
	EnvKey      string // Legacy config_env key name.
	TargetField string // Hydra field: "ca", "home", or "in".
	Destination string // For home: $HOME-relative path; for in: absolute path.
	Mode        string // For home: "ro" or ""; unused for ca/in.
}

// specialEnvMappings is the canonical mapping table. Order is stable and
// deterministic (sorted by key name) so migration output is reproducible.
var specialEnvMappings = []SpecialEnvMapping{
	{EnvKey: "CCR_CONFIG_JSON", TargetField: "home", Destination: ".claude-code-router/config.json", Mode: "ro"},
	{EnvKey: "CODEX_AUTH_JSON", TargetField: "home", Destination: ".codex/auth.json", Mode: "ro"},
	{EnvKey: "CODEX_CONFIG_TOML", TargetField: "home", Destination: ".codex/config.toml", Mode: "ro"},
	{EnvKey: "CODEX_PROMPT", TargetField: "in", Destination: "/in/codex-prompt.txt"},
	{EnvKey: "CRUSH_JSON", TargetField: "home", Destination: ".config/crush/crush.json", Mode: "ro"},
	{EnvKey: "PLOY_CA_CERTS", TargetField: "ca"},
}

// SpecialEnvMappingTable returns a copy of the canonical mapping table.
func SpecialEnvMappingTable() []SpecialEnvMapping {
	cp := make([]SpecialEnvMapping, len(specialEnvMappings))
	copy(cp, specialEnvMappings)
	return cp
}

// IsSpecialEnvKey reports whether key is a legacy special env key that should
// be migrated to a typed Hydra field.
func IsSpecialEnvKey(key string) bool {
	return LookupSpecialEnvMapping(key) != nil
}

// LookupSpecialEnvMapping returns the mapping for a special env key, or nil
// if the key is not a special env key.
func LookupSpecialEnvMapping(key string) *SpecialEnvMapping {
	for i := range specialEnvMappings {
		if specialEnvMappings[i].EnvKey == key {
			cp := specialEnvMappings[i]
			return &cp
		}
	}
	return nil
}

// MigrationAction describes the disposition of a scanned env record.
type MigrationAction string

const (
	// MigrationActionRewrite indicates the record should be rewritten to a
	// typed field after bundle upload.
	MigrationActionRewrite MigrationAction = "rewrite"
	// MigrationActionReject indicates the record conflicts with an existing
	// typed record and cannot be automatically migrated.
	MigrationActionReject MigrationAction = "reject"
	// MigrationActionSkip indicates the record's target does not map to job
	// sections (e.g., server or nodes target).
	MigrationActionSkip MigrationAction = "skip"
)

// MigrationReportEntry describes the migration status of a single env record.
type MigrationReportEntry struct {
	EnvKey      string          `json:"env_key"`
	Target      string          `json:"target"`
	Action      MigrationAction `json:"action"`
	TargetField string          `json:"target_field"`
	Destination string          `json:"destination,omitempty"`
	Sections    []string        `json:"sections,omitempty"`
	Reason      string          `json:"reason,omitempty"`
}

// MigrationReport summarizes a migration scan of special env keys.
type MigrationReport struct {
	Entries   []MigrationReportEntry `json:"entries"`
	Rewritten int                    `json:"rewritten"`
	Rejected  int                    `json:"rejected"`
	Skipped   int                    `json:"skipped"`
}

// sectionsForTarget returns the Hydra job sections that a GlobalEnvTarget
// maps to. Server and nodes targets return nil because they affect the
// process environment, not job containers.
func sectionsForTarget(target domaintypes.GlobalEnvTarget) []string {
	switch target {
	case domaintypes.GlobalEnvTargetGates:
		return []string{"pre_gate", "re_gate", "post_gate"}
	case domaintypes.GlobalEnvTargetSteps:
		return []string{"heal", "mig"}
	default:
		return nil
	}
}

// ScanSpecialEnvKeys examines global env entries and produces a migration
// report identifying special keys that should be migrated to typed fields.
//
// Records with server or nodes targets are skipped (they affect the process
// environment, not job containers). Records with gates or steps targets are
// candidates for migration to the corresponding Hydra sections.
//
// When existingHome contains records that would conflict with the migration
// target destination, the entry is rejected.
func ScanSpecialEnvKeys(
	globalEnv map[string][]GlobalEnvVar,
	existingCA map[string][]string,
	existingHome map[string][]ConfigHomeEntry,
) *MigrationReport {
	report := &MigrationReport{}

	keys := make([]string, 0, len(globalEnv))
	for k := range globalEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		mapping := LookupSpecialEnvMapping(key)
		if mapping == nil {
			continue
		}

		entries := globalEnv[key]
		for _, entry := range entries {
			sections := sectionsForTarget(entry.Target)
			if len(sections) == 0 {
				report.Entries = append(report.Entries, MigrationReportEntry{
					EnvKey:      key,
					Target:      entry.Target.String(),
					Action:      MigrationActionSkip,
					TargetField: mapping.TargetField,
					Reason:      fmt.Sprintf("target %q does not map to job sections", entry.Target),
				})
				report.Skipped++
				continue
			}

			var conflicts []string
			if mapping.TargetField == "home" {
				for _, section := range sections {
					for _, he := range existingHome[section] {
						if he.Dst == mapping.Destination {
							conflicts = append(conflicts, fmt.Sprintf(
								"section %q already has home entry for %q", section, mapping.Destination))
						}
					}
				}
			}

			if len(conflicts) > 0 {
				report.Entries = append(report.Entries, MigrationReportEntry{
					EnvKey:      key,
					Target:      entry.Target.String(),
					Action:      MigrationActionReject,
					TargetField: mapping.TargetField,
					Destination: mapping.Destination,
					Sections:    sections,
					Reason:      strings.Join(conflicts, "; "),
				})
				report.Rejected++
				continue
			}

			report.Entries = append(report.Entries, MigrationReportEntry{
				EnvKey:      key,
				Target:      entry.Target.String(),
				Action:      MigrationActionRewrite,
				TargetField: mapping.TargetField,
				Destination: mapping.Destination,
				Sections:    sections,
			})
			report.Rewritten++
		}
	}

	return report
}

// RewriteSpecialEnvEntry computes the canonical typed record string for a
// mapping given an uploaded content hash.
//
// For ca: returns ("ca", hash).
// For home: returns ("home", "hash:dst:ro") or ("home", "hash:dst").
// For in: returns ("in", "hash:dst").
func RewriteSpecialEnvEntry(mapping *SpecialEnvMapping, hash string) (field, entry string) {
	switch mapping.TargetField {
	case "ca":
		return "ca", hash
	case "home":
		if mapping.Mode == "ro" {
			return "home", hash + ":" + mapping.Destination + ":ro"
		}
		return "home", hash + ":" + mapping.Destination
	case "in":
		return "in", hash + ":" + mapping.Destination
	default:
		return mapping.TargetField, hash
	}
}

// LogMigrationReport logs the migration report at appropriate severity levels.
// Rewrite-needed entries are logged at Warn (action required), rejected entries
// at Error (conflict), and skipped entries at Info.
func LogMigrationReport(report *MigrationReport) {
	if len(report.Entries) == 0 {
		slog.Info("special env migration: no special env keys found")
		return
	}

	slog.Info("special env migration: scan complete",
		"total", len(report.Entries),
		"rewrite", report.Rewritten,
		"rejected", report.Rejected,
		"skipped", report.Skipped,
	)

	for _, e := range report.Entries {
		switch e.Action {
		case MigrationActionRewrite:
			slog.Warn("special env migration: rewrite needed",
				"key", e.EnvKey,
				"target", e.Target,
				"field", e.TargetField,
				"dst", e.Destination,
				"sections", e.Sections,
			)
		case MigrationActionReject:
			slog.Error("special env migration: conflict detected",
				"key", e.EnvKey,
				"target", e.Target,
				"field", e.TargetField,
				"reason", e.Reason,
			)
		case MigrationActionSkip:
			slog.Info("special env migration: skipped",
				"key", e.EnvKey,
				"target", e.Target,
				"reason", e.Reason,
			)
		}
	}
}

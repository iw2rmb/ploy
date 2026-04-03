package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
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
	existingIn map[string][]ConfigInEntry,
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
			switch mapping.TargetField {
			case "home":
				for _, section := range sections {
					for _, he := range existingHome[section] {
						if he.Dst == mapping.Destination {
							conflicts = append(conflicts, fmt.Sprintf(
								"section %q already has home entry for %q", section, mapping.Destination))
						}
					}
				}
			case "in":
				for _, section := range sections {
					for _, ie := range existingIn[section] {
						if ie.Dst == mapping.Destination {
							conflicts = append(conflicts, fmt.Sprintf(
								"section %q already has in entry for %q", section, mapping.Destination))
						}
					}
				}
			case "ca":
				// CA entries are content-addressed by hash. A collision occurs
				// when the same hash already exists in the target section. We
				// compute the hash from the env value to check.
				hash := contentHash(entry.Value)
				for _, section := range sections {
					for _, existing := range existingCA[section] {
						if existing == hash {
							conflicts = append(conflicts, fmt.Sprintf(
								"section %q already has ca entry for hash %s", section, hash[:12]))
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

// MigrationExecResult summarizes the outcome of executing a migration.
type MigrationExecResult struct {
	Persisted int // Number of typed records persisted.
	Deleted   int // Number of legacy env records removed.
	Rejected  int // Number of entries rejected (conflict).
	Skipped   int // Number of entries skipped (non-job target).
	Errors    []string
}

// contentHash computes a deterministic SHA-256 hex hash of the given value.
// This is used to derive the content-addressable identifier for migrated
// env values that become file-backed typed records.
func contentHash(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}

// ExecuteMigration persists rewrite-eligible report entries as typed ca/home/in
// records and removes the corresponding legacy env records from both the store
// and the in-memory ConfigHolder.
//
// Content hashes are computed from GlobalEnvVar values via SHA-256. The caller
// is responsible for ensuring that blob objects with matching hashes are
// available in object storage before nodes attempt materialization.
func ExecuteMigration(
	ctx context.Context,
	report *MigrationReport,
	st store.Store,
	holder *ConfigHolder,
) (*MigrationExecResult, error) {
	result := &MigrationExecResult{
		Rejected: report.Rejected,
		Skipped:  report.Skipped,
	}

	// Build a lookup from (envKey, target) → env value for hash computation.
	allEnv := holder.GetGlobalEnvAll()

	for _, entry := range report.Entries {
		if entry.Action != MigrationActionRewrite {
			continue
		}

		mapping := LookupSpecialEnvMapping(entry.EnvKey)
		if mapping == nil {
			result.Errors = append(result.Errors, fmt.Sprintf(
				"no mapping for key %q (internal error)", entry.EnvKey))
			continue
		}

		// Find the env value for this specific (key, target) pair.
		target, err := domaintypes.ParseGlobalEnvTarget(entry.Target)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf(
				"invalid target %q for key %q: %v", entry.Target, entry.EnvKey, err))
			continue
		}

		var envValue string
		var found bool
		for _, ev := range allEnv[entry.EnvKey] {
			if ev.Target == target {
				envValue = ev.Value
				found = true
				break
			}
		}
		if !found {
			result.Errors = append(result.Errors, fmt.Sprintf(
				"env value not found for key %q target %q", entry.EnvKey, entry.Target))
			continue
		}

		hash := contentHash(envValue)
		field, record := RewriteSpecialEnvEntry(mapping, hash)

		// Persist typed record to each target section.
		for _, section := range entry.Sections {
			if persistErr := persistTypedRecord(ctx, st, holder, field, record, section, mapping); persistErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf(
					"persist %s record for key %q section %q: %v",
					field, entry.EnvKey, section, persistErr))
				continue
			}
			result.Persisted++
		}

		// Delete legacy env record from store and holder.
		if deleteErr := st.DeleteGlobalEnv(ctx, store.DeleteGlobalEnvParams{
			Key:    entry.EnvKey,
			Target: target.String(),
		}); deleteErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf(
				"delete legacy env key %q target %q: %v",
				entry.EnvKey, entry.Target, deleteErr))
			continue
		}
		holder.DeleteGlobalEnvVar(entry.EnvKey, target)
		result.Deleted++
	}

	return result, nil
}

// persistTypedRecord persists a single typed record (ca, home, or in) to the
// store and updates the in-memory ConfigHolder.
func persistTypedRecord(
	ctx context.Context,
	st store.Store,
	holder *ConfigHolder,
	field, record, section string,
	mapping *SpecialEnvMapping,
) error {
	switch field {
	case "ca":
		if err := st.UpsertConfigCA(ctx, store.UpsertConfigCAParams{
			Hash:    record,
			Section: section,
		}); err != nil {
			return err
		}
		holder.AddConfigCA(section, record)

	case "home":
		dst := mapping.Destination
		if err := st.UpsertConfigHome(ctx, store.UpsertConfigHomeParams{
			Entry:   record,
			Dst:     dst,
			Section: section,
		}); err != nil {
			return err
		}
		holder.AddConfigHome(section, ConfigHomeEntry{
			Entry:   record,
			Dst:     dst,
			Section: section,
		})

	case "in":
		dst := mapping.Destination
		if err := st.UpsertConfigIn(ctx, store.UpsertConfigInParams{
			Entry:   record,
			Dst:     dst,
			Section: section,
		}); err != nil {
			return err
		}
		holder.AddConfigIn(section, ConfigInEntry{
			Entry:   record,
			Dst:     dst,
			Section: section,
		})

	default:
		return fmt.Errorf("unknown field %q", field)
	}
	return nil
}

// LogMigrationExecResult logs the execution result of a migration.
func LogMigrationExecResult(result *MigrationExecResult) {
	if result.Persisted == 0 && result.Deleted == 0 && result.Rejected == 0 && result.Skipped == 0 {
		slog.Info("special env migration: no special env keys found")
		return
	}

	slog.Info("special env migration: execution complete",
		"persisted", result.Persisted,
		"deleted", result.Deleted,
		"rejected", result.Rejected,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)

	for _, e := range result.Errors {
		slog.Error("special env migration: error", "detail", e)
	}
}

package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

var (
	migrationRecordsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ployd",
		Subsystem: "env_migration",
		Name:      "records_total",
		Help:      "Total special env migration records by disposition.",
	}, []string{"action"})

	migrationErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "ployd",
		Subsystem: "env_migration",
		Name:      "errors_total",
		Help:      "Total errors encountered during special env migration execution.",
	})
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

// allJobSections is the full set of Hydra sections across gates and steps.
var allJobSections = []string{"heal", "mig", "post_gate", "pre_gate", "re_gate"}

// sectionsForTarget returns the Hydra job sections that a GlobalEnvTarget
// maps to. Server and nodes targets map to all job sections because special
// env keys with these targets should be migrated to typed fields available
// to all job types (not applied as raw env vars).
func sectionsForTarget(target domaintypes.GlobalEnvTarget) []string {
	switch target {
	case domaintypes.GlobalEnvTargetGates:
		return []string{"pre_gate", "re_gate", "post_gate"}
	case domaintypes.GlobalEnvTargetSteps:
		return []string{"heal", "mig"}
	case domaintypes.GlobalEnvTargetServer, domaintypes.GlobalEnvTargetNodes:
		return allJobSections
	default:
		return nil
	}
}

// ScanSpecialEnvKeys examines global env entries and produces a migration
// report identifying special keys that should be migrated to typed fields.
//
// All targets (including server and nodes) are candidates for migration.
// When existingHome/In/CA contains records that conflict with the migration
// target, the entry is rejected — unless the existing record matches what
// the migration would produce (idempotent retry after partial failure).
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

			// Compute the expected migration output so we can distinguish
			// genuine conflicts from idempotent retries after partial failure.
			expectedHash := migrationExpectedHash(mapping, entry.Value)
			_, expectedRecord := RewriteSpecialEnvEntry(mapping, expectedHash)

			var conflicts []string
			switch mapping.TargetField {
			case "home":
				for _, section := range sections {
					for _, he := range existingHome[section] {
						if he.Dst == mapping.Destination && he.Entry != expectedRecord {
							conflicts = append(conflicts, fmt.Sprintf(
								"section %q already has home entry for %q", section, mapping.Destination))
						}
					}
				}
			case "in":
				for _, section := range sections {
					for _, ie := range existingIn[section] {
						if ie.Dst == mapping.Destination && ie.Entry != expectedRecord {
							conflicts = append(conflicts, fmt.Sprintf(
								"section %q already has in entry for %q", section, mapping.Destination))
						}
					}
				}
			case "ca":
				for _, section := range sections {
					for _, existing := range existingCA[section] {
						if existing == expectedHash {
							// Same hash from a prior migration run — not a conflict.
							continue
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

// LogMigrationReport logs the migration report at appropriate severity levels
// and emits Prometheus metrics for scan outcomes.
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

	// Emit Prometheus metrics for scan-phase outcomes.
	migrationRecordsTotal.WithLabelValues("rewrite_pending").Add(float64(report.Rewritten))
	migrationRecordsTotal.WithLabelValues("rejected").Add(float64(report.Rejected))
	migrationRecordsTotal.WithLabelValues("skipped").Add(float64(report.Skipped))
}

// MigrationExecResult summarizes the outcome of executing a migration.
type MigrationExecResult struct {
	Persisted int // Number of typed records persisted.
	Deleted   int // Number of legacy env records removed.
	Rejected  int // Number of entries rejected (conflict).
	Skipped   int // Number of entries skipped (non-job target).
	Errors    []string
}

// migrationShortHashLen is the fixed prefix length for canonical short hashes
// (12 hex chars), matching the compile-time convention.
const migrationShortHashLen = 12

// buildValueArchive wraps a raw env value in a deterministic tar.gz archive
// (single file named "content") matching the format produced by the CLI
// compile path. Returns the archive bytes.
func buildValueArchive(value string) ([]byte, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	data := []byte(value)
	hdr := &tar.Header{
		Name:     "content",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, fmt.Errorf("write tar header: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return nil, fmt.Errorf("write tar data: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}
	return buf.Bytes(), nil
}

// archiveShortHash computes the SHA-256 of archive bytes and returns the
// short hash prefix (first 12 hex chars), matching the compile-time convention.
func archiveShortHash(archiveBytes []byte) string {
	h := sha256.Sum256(archiveBytes)
	return hex.EncodeToString(h[:])[:migrationShortHashLen]
}

// contentHash computes a deterministic SHA-256 hex hash of the given value.
func contentHash(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}

// migrationExpectedHash computes the short hash that ExecuteMigration would
// store for a given mapping and env value. This is the archive-based short
// hash, matching the typed record key format used at execution time.
// Falls back to contentHash prefix on archive build failure (should not happen
// with valid values).
func migrationExpectedHash(_ *SpecialEnvMapping, value string) string {
	archiveBytes, err := buildValueArchive(value)
	if err != nil {
		return contentHash(value)[:migrationShortHashLen]
	}
	return archiveShortHash(archiveBytes)
}

// ExecuteMigration persists rewrite-eligible report entries as typed ca/home/in
// records and removes the corresponding legacy env records from both the store
// and the in-memory ConfigHolder.
//
// Each migrated env value is wrapped in a deterministic tar.gz archive and
// uploaded as a spec bundle so that node-side materialization can resolve the
// content hash via bundleMap. The short hash (first 12 hex chars of the
// archive's SHA-256) is used as the typed record key.
func ExecuteMigration(
	ctx context.Context,
	report *MigrationReport,
	st store.Store,
	holder *ConfigHolder,
	bp *blobpersist.Service,
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

		// Build a deterministic archive and upload as a spec bundle so
		// nodes can resolve the hash via bundleMap during materialization.
		archiveBytes, err := buildValueArchive(envValue)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf(
				"build archive for key %q target %q: %v", entry.EnvKey, entry.Target, err))
			continue
		}

		hash := archiveShortHash(archiveBytes)

		cid, digest := computeSpecBundleCIDAndDigest(archiveBytes)
		bundleID := domaintypes.NewSpecBundleID()
		createdBy := "env-migration"
		if _, uploadErr := bp.CreateSpecBundle(ctx, store.CreateSpecBundleParams{
			ID:        string(bundleID),
			Cid:       cid,
			Digest:    digest,
			Size:      int64(len(archiveBytes)),
			CreatedBy: &createdBy,
		}, archiveBytes); uploadErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf(
				"upload spec bundle for key %q target %q: %v", entry.EnvKey, entry.Target, uploadErr))
			continue
		}

		// Persist the hash → bundleID mapping to the store so it survives
		// server restarts, then update the in-memory ConfigHolder.
		if upsertErr := st.UpsertConfigBundleMap(ctx, store.UpsertConfigBundleMapParams{
			Hash:     hash,
			BundleID: string(bundleID),
		}); upsertErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf(
				"persist bundle mapping for hash %q: %v", hash, upsertErr))
			continue
		}
		holder.AddBundleMapping(hash, string(bundleID))

		field, record := RewriteSpecialEnvEntry(mapping, hash)

		// Persist typed record to each target section. Track failures so we
		// only delete the legacy env row when ALL sections succeeded.
		allSectionsOK := true
		for _, section := range entry.Sections {
			if persistErr := persistTypedRecord(ctx, st, holder, field, record, section, mapping); persistErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf(
					"persist %s record for key %q section %q: %v",
					field, entry.EnvKey, section, persistErr))
				allSectionsOK = false
				continue
			}
			result.Persisted++
		}

		// Only delete the legacy env record when every section was written
		// successfully. Partial writes leave the source intact so the next
		// startup can retry without data loss.
		if !allSectionsOK {
			result.Errors = append(result.Errors, fmt.Sprintf(
				"skipping legacy delete for key %q target %q: not all sections persisted",
				entry.EnvKey, entry.Target))
			continue
		}

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

// LogMigrationExecResult logs the execution result and emits Prometheus metrics.
func LogMigrationExecResult(result *MigrationExecResult) {
	if result.Persisted == 0 && result.Deleted == 0 && result.Rejected == 0 && result.Skipped == 0 && len(result.Errors) == 0 {
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

	// Emit Prometheus metrics for migration outcomes.
	migrationRecordsTotal.WithLabelValues("persisted").Add(float64(result.Persisted))
	migrationRecordsTotal.WithLabelValues("deleted").Add(float64(result.Deleted))
	migrationRecordsTotal.WithLabelValues("rejected").Add(float64(result.Rejected))
	migrationRecordsTotal.WithLabelValues("skipped").Add(float64(result.Skipped))
	migrationErrorsTotal.Add(float64(len(result.Errors)))
}

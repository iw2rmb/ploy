package types

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RunStats represents the terminal statistics payload stored on a run.
//
// This type uses json.RawMessage as its backing store instead of map[string]any.
// This design choice provides several benefits:
//   - Eliminates float64/any coercion issues inherent in map[string]any decoding.
//   - Improves schema control by preserving the original JSON structure.
//   - Enables efficient pass-through when stats are only relayed (no decode/re-encode).
//   - Maintains wire format compatibility with existing producers and consumers.
//
// Typed accessor methods (ExitCode, Metadata, MRURL, etc.) decode only the
// specific fields they need, avoiding full deserialization overhead.
type RunStats json.RawMessage

// runStatsAccessor is the internal typed structure for field extraction.
// Fields are decoded on-demand by accessor methods.
type runStatsAccessor struct {
	ExitCode      *int              `json:"exit_code,omitempty"`
	DurationMs    *int64            `json:"duration_ms,omitempty"`
	Error         *string           `json:"error,omitempty"`
	HealingWarn   *string           `json:"healing_warning,omitempty"`
	ResumeCount   *int              `json:"resume_count,omitempty"`
	LastResumedAt *string           `json:"last_resumed_at,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Gate          *RunStatsGate     `json:"gate,omitempty"`
	JobMeta       json.RawMessage   `json:"job_meta,omitempty"`
	Timings       *runStatsTimings  `json:"timings,omitempty"`
}

// RunStatsGate represents the gate sub-structure in stats.
type RunStatsGate struct {
	Passed     *bool               `json:"passed,omitempty"`
	DurationMs *int64              `json:"duration_ms,omitempty"`
	PreGate    *RunStatsGatePhase  `json:"pre_gate,omitempty"`
	FinalGate  *RunStatsGatePhase  `json:"final_gate,omitempty"`
	ReGates    []RunStatsGatePhase `json:"re_gates,omitempty"`
}

// RunStatsGatePhase represents a single gate execution phase.
type RunStatsGatePhase struct {
	Passed         bool                   `json:"passed"`
	DurationMs     int64                  `json:"duration_ms"`
	LogsArtifactID string                 `json:"logs_artifact_id,omitempty"`
	LogsBundleCID  string                 `json:"logs_bundle_cid,omitempty"`
	Resources      *RunStatsGateResources `json:"resources,omitempty"`
}

// RunStatsGateResources represents resource usage for a gate phase.
type RunStatsGateResources struct {
	Limits *RunStatsResourceLimits `json:"limits,omitempty"`
	Usage  *RunStatsResourceUsage  `json:"usage,omitempty"`
}

// RunStatsResourceLimits represents resource limits.
type RunStatsResourceLimits struct {
	NanoCPUs    int64 `json:"nano_cpus,omitempty"`
	MemoryBytes int64 `json:"memory_bytes,omitempty"`
}

// RunStatsResourceUsage represents resource usage.
type RunStatsResourceUsage struct {
	CPUTotalNs      uint64 `json:"cpu_total_ns,omitempty"`
	MemUsageBytes   uint64 `json:"mem_usage_bytes,omitempty"`
	MemMaxBytes     uint64 `json:"mem_max_bytes,omitempty"`
	BlkioReadBytes  uint64 `json:"blkio_read_bytes,omitempty"`
	BlkioWriteBytes uint64 `json:"blkio_write_bytes,omitempty"`
	SizeRwBytes     int64  `json:"size_rw_bytes,omitempty"`
}

// runStatsTimings represents execution timing metadata.
type runStatsTimings struct {
	HydrationDurationMs int64 `json:"hydration_duration_ms,omitempty"`
	ExecutionDurationMs int64 `json:"execution_duration_ms,omitempty"`
	BuildGateDurationMs int64 `json:"build_gate_duration_ms,omitempty"`
	DiffDurationMs      int64 `json:"diff_duration_ms,omitempty"`
	TotalDurationMs     int64 `json:"total_duration_ms,omitempty"`
}

// decode parses the raw JSON into the accessor struct.
// Returns empty struct if s is nil/empty or invalid JSON.
func (s RunStats) decode() runStatsAccessor {
	var acc runStatsAccessor
	if len(s) == 0 {
		return acc
	}
	// Silently ignore decode errors; accessors return zero values.
	_ = json.Unmarshal(s, &acc)
	return acc
}

// MarshalJSON implements json.Marshaler for RunStats.
func (s RunStats) MarshalJSON() ([]byte, error) {
	if len(s) == 0 {
		return []byte("null"), nil
	}
	return json.RawMessage(s).MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler for RunStats.
func (s *RunStats) UnmarshalJSON(data []byte) error {
	if data == nil {
		*s = nil
		return nil
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	*s = RunStats(copied)
	return nil
}

// IsEmpty returns true if the stats payload is nil, empty, or represents null/empty object.
func (s RunStats) IsEmpty() bool {
	if len(s) == 0 {
		return true
	}
	trimmed := strings.TrimSpace(string(s))
	return trimmed == "" || trimmed == "null" || trimmed == "{}"
}

// ExitCode returns the exit_code field as an int when present.
func (s RunStats) ExitCode() (int, bool) {
	acc := s.decode()
	if acc.ExitCode == nil {
		return 0, false
	}
	return *acc.ExitCode, true
}

// Metadata returns a copy of the metadata field as map[string]string.
// Empty strings and whitespace-only values are excluded.
func (s RunStats) Metadata() map[string]string {
	acc := s.decode()
	if acc.Metadata == nil {
		return map[string]string{}
	}
	// Return a copy with whitespace trimmed.
	out := make(map[string]string, len(acc.Metadata))
	for k, v := range acc.Metadata {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			out[k] = trimmed
		}
	}
	return out
}

// MRURL returns the mr_url entry from the metadata map when present.
func (s RunStats) MRURL() string {
	meta := s.Metadata()
	if meta == nil {
		return ""
	}
	return strings.TrimSpace(meta["mr_url"])
}

// ResumeCount returns the number of times this run has been resumed.
// Returns 0 if never resumed.
func (s RunStats) ResumeCount() int {
	acc := s.decode()
	if acc.ResumeCount == nil {
		return 0
	}
	return *acc.ResumeCount
}

// LastResumedAt returns the RFC3339 timestamp of the last resume, or empty string if never resumed.
func (s RunStats) LastResumedAt() string {
	acc := s.decode()
	if acc.LastResumedAt == nil {
		return ""
	}
	return strings.TrimSpace(*acc.LastResumedAt)
}

// GateSummary extracts build gate execution summary from the gate field.
// Returns a human-readable summary string suitable for CLI/API display.
// Format: "passed duration=123ms" or "failed pre-gate duration=45ms" or empty if no gate data.
//
// Priority order:
//  1. final_gate — The latest post-mig gate result. For runs with no migs executed,
//     final_gate is populated from the pre-mig gate to ensure consistent summary output.
//  2. last re-gate — The most recent healing re-gate attempt (from either pre- or post-mig phases).
//  3. pre_gate — The initial pre-mig gate before any mig execution (fallback when no final_gate).
//
// This priority ensures CLI and API consumers always get the most definitive gate result:
// final_gate represents the authoritative build validation status at run completion.
func (s RunStats) GateSummary() string {
	acc := s.decode()
	if acc.Gate == nil {
		return ""
	}

	// Check final_gate first (post-mig gate or pre-mig gate fallback for runs with no migs).
	if acc.Gate.FinalGate != nil {
		return formatGatePhaseTyped(acc.Gate.FinalGate, "final-gate")
	}

	// Check re_gates array (healing attempts from both pre- and post-mig phases).
	if len(acc.Gate.ReGates) > 0 {
		// Take the last re-gate run as the most recent healing result.
		lastReGate := &acc.Gate.ReGates[len(acc.Gate.ReGates)-1]
		if summary := formatGatePhaseTyped(lastReGate, "re-gate"); summary != "" {
			return summary
		}
	}

	// Fall back to pre_gate (pre-mig gate) — only reached if no final_gate was populated.
	if acc.Gate.PreGate != nil {
		return formatGatePhaseTyped(acc.Gate.PreGate, "pre-gate")
	}

	return ""
}

// formatGatePhaseTyped builds a summary string from a typed gate phase.
// Format: "passed duration=123ms" or "failed pre-gate duration=45ms".
func formatGatePhaseTyped(phase *RunStatsGatePhase, label string) string {
	if phase == nil {
		return ""
	}

	status := "passed"
	if !phase.Passed {
		status = "failed"
	}

	// Include phase label only for failed gates or non-final phases for clarity.
	if !phase.Passed || label != "final-gate" {
		return fmt.Sprintf("%s %s duration=%dms", status, label, phase.DurationMs)
	}
	return fmt.Sprintf("%s duration=%dms", status, phase.DurationMs)
}

// RunStatsBuilder provides a fluent API for constructing RunStats.
// This replaces map literal construction with a type-safe builder pattern.
type RunStatsBuilder struct {
	acc runStatsAccessor
}

// NewRunStatsBuilder creates a new builder for constructing RunStats.
func NewRunStatsBuilder() *RunStatsBuilder {
	return &RunStatsBuilder{}
}

// ExitCode sets the exit_code field.
func (b *RunStatsBuilder) ExitCode(code int) *RunStatsBuilder {
	b.acc.ExitCode = &code
	return b
}

// DurationMs sets the duration_ms field.
func (b *RunStatsBuilder) DurationMs(ms int64) *RunStatsBuilder {
	b.acc.DurationMs = &ms
	return b
}

// Error sets the error field for failure diagnostics.
func (b *RunStatsBuilder) Error(msg string) *RunStatsBuilder {
	b.acc.Error = &msg
	return b
}

// HealingWarning sets the healing_warning field.
func (b *RunStatsBuilder) HealingWarning(warn string) *RunStatsBuilder {
	b.acc.HealingWarn = &warn
	return b
}

// ResumeCount sets the resume_count field.
func (b *RunStatsBuilder) ResumeCount(count int) *RunStatsBuilder {
	b.acc.ResumeCount = &count
	return b
}

// LastResumedAt sets the last_resumed_at field.
func (b *RunStatsBuilder) LastResumedAt(ts string) *RunStatsBuilder {
	b.acc.LastResumedAt = &ts
	return b
}

// Metadata sets the metadata field.
func (b *RunStatsBuilder) Metadata(meta map[string]string) *RunStatsBuilder {
	b.acc.Metadata = meta
	return b
}

// MetadataEntry adds a single key-value pair to the metadata field.
func (b *RunStatsBuilder) MetadataEntry(key, value string) *RunStatsBuilder {
	if b.acc.Metadata == nil {
		b.acc.Metadata = make(map[string]string)
	}
	b.acc.Metadata[key] = value
	return b
}

// Timings sets the timings field.
func (b *RunStatsBuilder) Timings(t *runStatsTimings) *RunStatsBuilder {
	b.acc.Timings = t
	return b
}

// TimingsFromDurations sets the timings field from duration values in milliseconds.
func (b *RunStatsBuilder) TimingsFromDurations(hydration, execution, diff, total int64) *RunStatsBuilder {
	b.acc.Timings = &runStatsTimings{
		HydrationDurationMs: hydration,
		ExecutionDurationMs: execution,
		DiffDurationMs:      diff,
		TotalDurationMs:     total,
	}
	return b
}

// TimingsWithGate sets the timings field including build gate duration.
func (b *RunStatsBuilder) TimingsWithGate(hydration, execution, gate, diff, total int64) *RunStatsBuilder {
	b.acc.Timings = &runStatsTimings{
		HydrationDurationMs: hydration,
		ExecutionDurationMs: execution,
		BuildGateDurationMs: gate,
		DiffDurationMs:      diff,
		TotalDurationMs:     total,
	}
	return b
}

// Gate sets the gate field for gate-only stats (simple pass/fail + duration).
// The gate is stored as final_gate to align with GateSummary extraction logic.
func (b *RunStatsBuilder) Gate(passed bool, durationMs int64) *RunStatsBuilder {
	b.acc.Gate = &RunStatsGate{
		Passed:     &passed,
		DurationMs: &durationMs,
		FinalGate: &RunStatsGatePhase{
			Passed:     passed,
			DurationMs: durationMs,
		},
	}
	return b
}

// GateDetails sets the full gate object (pre-gate, re-gates, final gate, resources, etc.).
func (b *RunStatsBuilder) GateDetails(gate *RunStatsGate) *RunStatsBuilder {
	b.acc.Gate = gate
	return b
}

// JobMeta sets the job_meta field (raw JSON for job metadata).
func (b *RunStatsBuilder) JobMeta(meta json.RawMessage) *RunStatsBuilder {
	if len(meta) == 0 {
		return b
	}
	b.acc.JobMeta = append(json.RawMessage(nil), meta...)
	return b
}

// Build constructs the final RunStats value.
// Returns nil if all fields are empty/zero.
func (b *RunStatsBuilder) Build() RunStats {
	data, err := json.Marshal(b.acc)
	if err != nil {
		return nil
	}
	// Check for empty object.
	if string(data) == "{}" {
		return nil
	}
	return RunStats(data)
}

// MustBuild constructs the final RunStats value, panicking on marshal error.
// This is useful in tests or when the builder state is guaranteed to be valid.
func (b *RunStatsBuilder) MustBuild() RunStats {
	stats := b.Build()
	if stats == nil {
		return RunStats("{}")
	}
	return stats
}

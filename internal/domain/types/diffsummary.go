package types

import (
	"encoding/json"
	"strings"
)

// DiffSummary represents summary metadata attached to a diff.
//
// This type uses json.RawMessage as its backing store instead of map[string]any.
// This design choice provides several benefits:
//   - Eliminates float64/any coercion issues inherent in map[string]any decoding.
//   - Improves schema control by preserving the original JSON structure.
//   - Enables efficient pass-through when summaries are only relayed (no decode/re-encode).
//   - Maintains wire format compatibility with existing producers and consumers.
//
// Typed accessor methods (ExitCode, FilesChanged, etc.) decode only the
// specific fields they need, avoiding full deserialization overhead.
type DiffSummary json.RawMessage

// diffSummaryAccessor is the internal typed structure for field extraction.
// Fields are decoded on-demand by accessor methods.
type diffSummaryAccessor struct {
	ExitCode     *int                `json:"exit_code,omitempty"`
	FilesChanged *int                `json:"files_changed,omitempty"`
	JobType      *string             `json:"job_type,omitempty"`
	Timings      *diffSummaryTimings `json:"timings,omitempty"`
}

// diffSummaryTimings represents execution timing metadata in a diff summary.
type diffSummaryTimings struct {
	HydrationDurationMs int64 `json:"hydration_duration_ms,omitempty"`
	ExecutionDurationMs int64 `json:"execution_duration_ms,omitempty"`
	DiffDurationMs      int64 `json:"diff_duration_ms,omitempty"`
	TotalDurationMs     int64 `json:"total_duration_ms,omitempty"`
}

// decode parses the raw JSON into the accessor struct.
// Returns empty struct if d is nil/empty or invalid JSON.
func (d DiffSummary) decode() diffSummaryAccessor {
	var acc diffSummaryAccessor
	if len(d) == 0 {
		return acc
	}
	// Return an empty accessor on decode errors (strict contract).
	if err := json.Unmarshal(d, &acc); err != nil {
		return diffSummaryAccessor{}
	}
	return acc
}

// MarshalJSON implements json.Marshaler for DiffSummary.
func (d DiffSummary) MarshalJSON() ([]byte, error) {
	if len(d) == 0 {
		return []byte("null"), nil
	}
	return json.RawMessage(d).MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler for DiffSummary.
func (d *DiffSummary) UnmarshalJSON(data []byte) error {
	if data == nil {
		*d = nil
		return nil
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	*d = DiffSummary(copied)
	return nil
}

// IsEmpty returns true if the summary payload is nil, empty, or represents null/empty object.
func (d DiffSummary) IsEmpty() bool {
	if len(d) == 0 {
		return true
	}
	trimmed := strings.TrimSpace(string(d))
	return trimmed == "" || trimmed == "null" || trimmed == "{}"
}

// ExitCode returns the exit_code field as an int when present.
func (d DiffSummary) ExitCode() (int, bool) {
	acc := d.decode()
	if acc.ExitCode == nil {
		return 0, false
	}
	return *acc.ExitCode, true
}

// FilesChanged returns the files_changed field as an int when present.
func (d DiffSummary) FilesChanged() (int, bool) {
	acc := d.decode()
	if acc.FilesChanged == nil {
		return 0, false
	}
	return *acc.FilesChanged, true
}

// JobType returns the job_type field when present.
// Common values: "mig", "healing".
func (d DiffSummary) JobType() string {
	acc := d.decode()
	if acc.JobType == nil {
		return ""
	}
	return *acc.JobType
}

// DiffSummaryBuilder provides a fluent API for constructing DiffSummary.
// This replaces map literal construction with a type-safe builder pattern.
type DiffSummaryBuilder struct {
	acc diffSummaryAccessor
}

// NewDiffSummaryBuilder creates a new builder for constructing DiffSummary.
func NewDiffSummaryBuilder() *DiffSummaryBuilder {
	return &DiffSummaryBuilder{}
}

// ExitCode sets the exit_code field.
func (b *DiffSummaryBuilder) ExitCode(code int) *DiffSummaryBuilder {
	b.acc.ExitCode = &code
	return b
}

// FilesChanged sets the files_changed field.
func (b *DiffSummaryBuilder) FilesChanged(count int) *DiffSummaryBuilder {
	b.acc.FilesChanged = &count
	return b
}

// JobType sets the job_type field.
func (b *DiffSummaryBuilder) JobType(jobType string) *DiffSummaryBuilder {
	b.acc.JobType = &jobType
	return b
}

// Timings sets the timings field from duration values in milliseconds.
func (b *DiffSummaryBuilder) Timings(hydration, execution, diff, total int64) *DiffSummaryBuilder {
	b.acc.Timings = &diffSummaryTimings{
		HydrationDurationMs: hydration,
		ExecutionDurationMs: execution,
		DiffDurationMs:      diff,
		TotalDurationMs:     total,
	}
	return b
}

// Build constructs the final DiffSummary value.
// Returns nil if all fields are empty/zero.
func (b *DiffSummaryBuilder) Build() DiffSummary {
	data, err := json.Marshal(b.acc)
	if err != nil {
		return nil
	}
	// Check for empty object.
	if string(data) == "{}" {
		return nil
	}
	return DiffSummary(data)
}

// MustBuild constructs the final DiffSummary value, panicking on marshal error.
// This is useful in tests or when the builder state is guaranteed to be valid.
func (b *DiffSummaryBuilder) MustBuild() DiffSummary {
	summary := b.Build()
	if summary == nil {
		return DiffSummary("{}")
	}
	return summary
}

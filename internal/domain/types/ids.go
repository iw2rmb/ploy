package types

// This file defines stable identifier types used across the system.
// IDs are simple string newtypes that marshal to/from JSON as strings
// and reject empty or whitespace-only values when decoded from text.

import (
	"encoding"
	"math"
)

// RunID identifies a run instance (workflow execution).
// This is the canonical identifier for workflow runs in the Mods system.
type RunID string

// StepID identifies a step within a stage.
type StepID string

// JobID identifies a job within the execution context.
// Jobs are the unit of work assignment to nodes (claim, execute, complete).
type JobID string

// ClusterID identifies a CLI/server cluster descriptor.
type ClusterID string

// NodeID identifies a worker node in the cluster.
type NodeID string

// RunRepoID identifies a repo entry within a batch run.
// Used in batch run handlers to provide type-safe repo identification.
type RunRepoID string

// ModID identifies a mod project.
// v1 identifier for mods table (NanoID(6), app-generated).
// See roadmap/v1/db.md:15 for schema requirements.
type ModID string

// SpecID identifies a spec instance in the specs table.
// v1 identifier for specs table (NanoID(8), app-generated).
// See roadmap/v1/db.md:44 for schema requirements.
type SpecID string

// ModRepoID identifies a repo entry within a mod project.
// v1 identifier for mod_repos table (NanoID(8), app-generated).
// See roadmap/v1/db.md:62 for schema requirements.
// Note: In CLI flags and some legacy naming, this may be called "mod_repo_id".
// Prefer "repo_id" in v1 docs per roadmap/v1/db.md:98.
type ModRepoID string

// StepIndex identifies a job's ordering value within a run execution sequence.
// It is stored/transported as a float64 (historically `jobs.step_index`) where
// integer-like values (e.g., 1000, 2000, 1500) encode ordering.
type StepIndex float64

// String returns the underlying string value.
func (v RunID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v RunID) IsZero() bool { return IsEmpty(string(v)) }

// String returns the underlying string value.
func (v StepID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v StepID) IsZero() bool { return IsEmpty(string(v)) }

// String returns the underlying string value.
func (v JobID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v JobID) IsZero() bool { return IsEmpty(string(v)) }

// String returns the underlying string value.
func (v ClusterID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v ClusterID) IsZero() bool { return IsEmpty(string(v)) }

// String returns the underlying string value.
func (v NodeID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v NodeID) IsZero() bool { return IsEmpty(string(v)) }

// String returns the underlying string value.
func (v RunRepoID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v RunRepoID) IsZero() bool { return IsEmpty(string(v)) }

// String returns the underlying string value.
func (v ModID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v ModID) IsZero() bool { return IsEmpty(string(v)) }

// String returns the underlying string value.
func (v SpecID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v SpecID) IsZero() bool { return IsEmpty(string(v)) }

// String returns the underlying string value.
func (v ModRepoID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v ModRepoID) IsZero() bool { return IsEmpty(string(v)) }

// Float64 returns the underlying float64 value.
func (v StepIndex) Float64() float64 { return float64(v) }

// IsZero reports whether the step index is zero.
func (v StepIndex) IsZero() bool { return v == 0 }

// Valid reports whether the StepIndex represents a valid step ordering value.
// A valid StepIndex must:
//   - Not be NaN or Inf (rejects special floating-point values)
//   - Have a zero fractional part (e.g., 1000.0 is valid, 1000.5 is not)
//
// This centralizes invariants for step indices, ensuring they represent
// discrete positions in the execution sequence (e.g., 1000, 2000, 1500).
func (v StepIndex) Valid() bool {
	f := float64(v)
	// Reject NaN and Inf.
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return false
	}
	// Require integer-like value (no fractional part).
	return f == math.Trunc(f)
}

// The following methods implement encoding.TextMarshaler/TextUnmarshaler and
// JSON helpers for each ID type. Using small helpers avoids duplication while
// keeping explicit method sets for clarity and future extension.

func marshalIDText[S ~string](v S) ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	return []byte(s), nil
}

func unmarshalIDText[S ~string](dst *S, b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	*dst = S(s)
	return nil
}

// RunID implements encoding.TextMarshaler and encoding.TextUnmarshaler
// for text-based serialization (normalizes and rejects empty values).
var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*RunID)(nil)

func (v RunID) MarshalText() ([]byte, error)  { return marshalIDText(v) }
func (v *RunID) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }
func (v RunID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *RunID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*StepID)(nil)

func (v StepID) MarshalText() ([]byte, error)  { return marshalIDText(v) }
func (v *StepID) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }
func (v StepID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *StepID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*JobID)(nil)

func (v JobID) MarshalText() ([]byte, error)  { return marshalIDText(v) }
func (v *JobID) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }
func (v JobID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *JobID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*ClusterID)(nil)

func (v ClusterID) MarshalText() ([]byte, error)  { return marshalIDText(v) }
func (v *ClusterID) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }
func (v ClusterID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *ClusterID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*NodeID)(nil)

func (v NodeID) MarshalText() ([]byte, error)  { return marshalIDText(v) }
func (v *NodeID) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }
func (v NodeID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *NodeID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*RunRepoID)(nil)

func (v RunRepoID) MarshalText() ([]byte, error)  { return marshalIDText(v) }
func (v *RunRepoID) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }
func (v RunRepoID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *RunRepoID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// ModID implements encoding.TextMarshaler and encoding.TextUnmarshaler
// for text-based serialization (normalizes and rejects empty values).
var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*ModID)(nil)

func (v ModID) MarshalText() ([]byte, error)  { return marshalIDText(v) }
func (v *ModID) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }
func (v ModID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *ModID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// SpecID implements encoding.TextMarshaler and encoding.TextUnmarshaler
// for text-based serialization (normalizes and rejects empty values).
var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*SpecID)(nil)

func (v SpecID) MarshalText() ([]byte, error)  { return marshalIDText(v) }
func (v *SpecID) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }
func (v SpecID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *SpecID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// ModRepoID implements encoding.TextMarshaler and encoding.TextUnmarshaler
// for text-based serialization (normalizes and rejects empty values).
var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*ModRepoID)(nil)

func (v ModRepoID) MarshalText() ([]byte, error)  { return marshalIDText(v) }
func (v *ModRepoID) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }
func (v ModRepoID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *ModRepoID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// StepIndex uses standard float64 JSON marshaling (not text-based like string IDs).

package types

// This file defines stable identifier types used across the system.
// IDs are simple string newtypes that marshal to/from JSON as strings
// and reject empty or whitespace-only values when decoded from text.

import "encoding"

// RunID identifies a run instance (workflow execution).
// This is the canonical identifier for workflow runs, consolidating
// the previous TicketID and RunID types into a single type.
type RunID string

// TicketID is a deprecated alias for RunID.
// Use RunID directly for all new code. This alias exists only for
// backward compatibility during the migration from ticket to run terminology.
//
// Deprecated: Use RunID instead. This alias will be removed once all
// callers have been migrated (see ROADMAP.md tasks).
type TicketID = RunID

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

// StepIndex identifies a step's position within a job execution sequence.
// It is a zero-based index representing the order of execution.
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

// Float64 returns the underlying float64 value.
func (v StepIndex) Float64() float64 { return float64(v) }

// IsZero reports whether the step index is zero.
func (v StepIndex) IsZero() bool { return v == 0 }

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
// Note: TicketID is a type alias for RunID and shares these methods.
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

// StepIndex uses standard float64 JSON marshaling (not text-based like string IDs).

package types

// This file defines stable identifier types used across the system.
// IDs are simple string newtypes that marshal to/from JSON as strings
// and reject empty or whitespace-only values when decoded from text.

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
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

// ModID identifies a mod project.
// Uses NanoID(6) for compact, URL-safe identifiers suitable for CLI usage and display.
type ModID string

// SpecID identifies a spec instance in the specs table.
// Uses NanoID(8) for spec identifiers in the append-only specs table.
type SpecID string

// ModRepoID identifies a repo entry within a mod project.
// Uses NanoID(8) for per-mod repository identifiers.
// Note: In CLI flags and some legacy naming, this may be called "mod_repo_id".
// Prefer "repo_id" in API contexts.
type ModRepoID string

// ModRef is a reference that can be either a mod ID or a mod name.
// Used for endpoints that accept "mod id OR name" in the path.
// This type prevents conflating IDs with names at the type level.
// Values must be non-empty and URL-safe (no whitespace, no / or ? characters).
type ModRef string

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

// String returns the underlying string value.
func (v ModRef) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v ModRef) IsZero() bool { return IsEmpty(string(v)) }

// Validate checks that the ModRef is non-empty and URL-safe.
// URL-safe means no whitespace and no / or ? characters.
func (v ModRef) Validate() error {
	s := Normalize(string(v))
	if s == "" {
		return ErrEmpty
	}
	for _, c := range s {
		if c == '/' || c == '?' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			return ErrInvalidModRef
		}
	}
	return nil
}

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

// ModRef implements encoding.TextMarshaler and encoding.TextUnmarshaler
// for text-based serialization (normalizes and validates URL-safety).
var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*ModRef)(nil)

func (v ModRef) MarshalText() ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	return []byte(Normalize(string(v))), nil
}

func (v *ModRef) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	ref := ModRef(s)
	if err := ref.Validate(); err != nil {
		return err
	}
	*v = ref
	return nil
}

func (v ModRef) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *ModRef) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// StepIndex uses standard float64 JSON marshaling (not text-based like string IDs).

// EventID identifies an SSE event in a stream for resumption semantics.
// The value must be non-negative; zero is a valid ID representing "from the beginning".
// Negative values are invalid and should be rejected at boundaries (e.g., header parsing).
type EventID int64

// Int64 returns the underlying int64 value.
func (v EventID) Int64() int64 { return int64(v) }

// IsZero reports whether the event ID is zero.
func (v EventID) IsZero() bool { return v == 0 }

// Valid reports whether the EventID represents a valid SSE cursor.
// A valid EventID must be non-negative (>= 0).
func (v EventID) Valid() bool { return v >= 0 }

// String returns the decimal string representation of the event ID.
func (v EventID) String() string { return strconv.FormatInt(int64(v), 10) }

// EventID implements encoding.TextMarshaler and encoding.TextUnmarshaler
// for text-based serialization (Last-Event-ID header, etc.).
var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*EventID)(nil)

// MarshalText encodes the EventID as a decimal string.
// Returns an error if the value is invalid (negative).
func (v EventID) MarshalText() ([]byte, error) {
	if !v.Valid() {
		return nil, errors.New("types: invalid EventID (negative)")
	}
	return []byte(v.String()), nil
}

// UnmarshalText decodes a decimal string into an EventID.
// Returns an error if the string is empty, not a valid integer, or negative.
func (v *EventID) UnmarshalText(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return errors.New("types: empty EventID")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("types: invalid EventID %q: %w", s, err)
	}
	if n < 0 {
		return fmt.Errorf("types: invalid EventID %d (negative)", n)
	}
	*v = EventID(n)
	return nil
}

// MarshalJSON encodes the EventID as a JSON number.
func (v EventID) MarshalJSON() ([]byte, error) {
	if !v.Valid() {
		return nil, errors.New("types: invalid EventID (negative)")
	}
	return json.Marshal(int64(v))
}

// UnmarshalJSON decodes a JSON number into an EventID.
func (v *EventID) UnmarshalJSON(b []byte) error {
	if strings.TrimSpace(string(b)) == "null" {
		return errors.New("types: invalid EventID JSON null")
	}
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("types: invalid EventID JSON %q: %w", string(b), err)
	}
	if n < 0 {
		return fmt.Errorf("types: invalid EventID %d (negative)", n)
	}
	*v = EventID(n)
	return nil
}

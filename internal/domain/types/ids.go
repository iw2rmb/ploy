package types

// This file defines stable identifier types used across the system.
// IDs are simple string newtypes that marshal to/from JSON as strings
// and reject empty or whitespace-only values when decoded from text.

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
)

// IDValidator is implemented by tag types that define format validation for an ID.
type IDValidator interface {
	ValidateID(string) error
}

// StringID is a generic string identifier type. The tag type T determines
// validation behavior: if T implements IDValidator, its ValidateID method
// is used during text marshaling/unmarshaling; otherwise no validation is applied.
type StringID[T any] string

func (v StringID[T]) String() string { return string(v) }
func (v StringID[T]) IsZero() bool   { return IsEmpty(string(v)) }

func (v StringID[T]) MarshalText() ([]byte, error) {
	return marshalIDTextValidated(v, idValidator[T]())
}

func (v *StringID[T]) UnmarshalText(b []byte) error {
	return unmarshalIDTextValidated(v, b, idValidator[T]())
}

func (v StringID[T]) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *StringID[T]) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
	json.Marshaler
	json.Unmarshaler
} = (*StringID[struct{}])(nil)

// idValidator returns the validation function for tag type T, or nil if T
// does not implement IDValidator.
func idValidator[T any]() func(string) error {
	var tag T
	if v, ok := any(tag).(IDValidator); ok {
		return v.ValidateID
	}
	return nil
}

// Tag types for each string ID. Types with format validation implement IDValidator.
type (
	runIDTag     struct{}
	stepIDTag    struct{}
	jobIDTag     struct{}
	clusterIDTag struct{}
	nodeIDTag    struct{}
	modIDTag     struct{}
	specIDTag    struct{}
	modRepoIDTag struct{}
)

func (runIDTag) ValidateID(s string) error     { return validateKSUID(s, ErrInvalidRunID) }
func (jobIDTag) ValidateID(s string) error     { return validateKSUID(s, ErrInvalidJobID) }
func (nodeIDTag) ValidateID(s string) error    { return validateNanoID(s, 6, ErrInvalidNodeID) }
func (modIDTag) ValidateID(s string) error     { return validateNanoID(s, 6, ErrInvalidModID) }
func (specIDTag) ValidateID(s string) error    { return validateNanoID(s, 8, ErrInvalidSpecID) }
func (modRepoIDTag) ValidateID(s string) error { return validateNanoID(s, 8, ErrInvalidModRepoID) }

// RunID identifies a run instance (workflow execution).
type RunID = StringID[runIDTag]

// StepID identifies a step within a stage.
type StepID = StringID[stepIDTag]

// JobID identifies a job within the execution context.
// Jobs are the unit of work assignment to nodes (claim, execute, complete).
type JobID = StringID[jobIDTag]

// ClusterID identifies a CLI/server cluster descriptor.
type ClusterID = StringID[clusterIDTag]

// NodeID identifies a worker node in the cluster.
type NodeID = StringID[nodeIDTag]

// ModID identifies a mod project.
// Uses NanoID(6) for compact, URL-safe identifiers suitable for CLI usage and display.
type ModID = StringID[modIDTag]

// SpecID identifies a spec instance in the specs table.
// Uses NanoID(8) for spec identifiers in the append-only specs table.
type SpecID = StringID[specIDTag]

// ModRepoID identifies a repo entry within a mod project.
// Uses NanoID(8) for per-mod repository identifiers.
type ModRepoID = StringID[modRepoIDTag]

// ModRef is a reference that can be either a mod ID or a mod name.
// Used for endpoints that accept "mod id OR name" in the path.
// This type prevents conflating IDs with names at the type level.
// Values must be non-empty and URL-safe (no whitespace, no / or ? characters).
type ModRef string

func (v ModRef) String() string { return string(v) }
func (v ModRef) IsZero() bool   { return IsEmpty(string(v)) }

// Validate checks that the ModRef is non-empty and URL-safe.
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

// Validation errors for ID types.
var (
	ErrInvalidRunID     = errors.New("invalid run id")
	ErrInvalidJobID     = errors.New("invalid job id")
	ErrInvalidNodeID    = errors.New("invalid node id")
	ErrInvalidModID     = errors.New("invalid mod id")
	ErrInvalidSpecID    = errors.New("invalid spec id")
	ErrInvalidModRepoID = errors.New("invalid mod repo id")
	ErrInvalidDiffID    = errors.New("invalid diff id")
)

// DiffID identifies a stored diff record.
// It is UUID-backed and must be a valid UUID string when decoded from text/JSON.
type DiffID string

// String returns the underlying string value.
func (v DiffID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v DiffID) IsZero() bool { return IsEmpty(string(v)) }

// Validate verifies the diff ID is non-empty and UUID-parseable.
func (v DiffID) Validate() error {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return ErrEmpty
	}
	if _, err := uuid.Parse(s); err != nil {
		return ErrInvalidDiffID
	}
	return nil
}

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*DiffID)(nil)

func (v DiffID) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	if _, err := uuid.Parse(s); err != nil {
		return nil, ErrInvalidDiffID
	}
	return []byte(s), nil
}

func (v *DiffID) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	if _, err := uuid.Parse(s); err != nil {
		return ErrInvalidDiffID
	}
	*v = DiffID(s)
	return nil
}

func (v DiffID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *DiffID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

var nanoIDAlphabetTable = func() [256]bool {
	var t [256]bool
	for i := 0; i < len(alphabet); i++ {
		t[alphabet[i]] = true
	}
	return t
}()

func validateKSUID(s string, errInvalid error) error {
	if _, err := ksuid.Parse(s); err != nil {
		return errInvalid
	}
	return nil
}

func validateNanoID(s string, length int, errInvalid error) error {
	if len(s) != length {
		return errInvalid
	}
	for i := 0; i < len(s); i++ {
		if !nanoIDAlphabetTable[s[i]] {
			return errInvalid
		}
	}
	return nil
}

func marshalIDText[S ~string](v S) ([]byte, error) {
	return marshalIDTextValidated(v, nil)
}

func unmarshalIDText[S ~string](dst *S, b []byte) error {
	return unmarshalIDTextValidated(dst, b, nil)
}

func marshalIDTextValidated[S ~string](v S, validate func(string) error) ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	if validate != nil {
		if err := validate(s); err != nil {
			return nil, err
		}
	}
	return []byte(s), nil
}

func unmarshalIDTextValidated[S ~string](dst *S, b []byte, validate func(string) error) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	if validate != nil {
		if err := validate(s); err != nil {
			return err
		}
	}
	*dst = S(s)
	return nil
}

// EventID identifies an SSE event in a stream for resumption semantics.
type EventID int64

func (v EventID) Int64() int64   { return int64(v) }
func (v EventID) IsZero() bool   { return v == 0 }
func (v EventID) Valid() bool    { return v >= 0 }
func (v EventID) String() string { return strconv.FormatInt(int64(v), 10) }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*EventID)(nil)

func (v EventID) MarshalText() ([]byte, error) {
	if !v.Valid() {
		return nil, errors.New("types: invalid EventID (negative)")
	}
	return []byte(v.String()), nil
}

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

func (v EventID) MarshalJSON() ([]byte, error) {
	if !v.Valid() {
		return nil, errors.New("types: invalid EventID (negative)")
	}
	return json.Marshal(int64(v))
}

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

// LabelRunID is the container label key storing the run identifier.
const LabelRunID = "com.ploy.run_id"

// LabelJobID is the container label key storing the job identifier.
const LabelJobID = "com.ploy.job_id"

// LabelsForRun returns a labels map containing the run identifier.
// When id is empty, it returns nil.
func LabelsForRun(id RunID) map[string]string {
	if id.IsZero() {
		return nil
	}
	return map[string]string{LabelRunID: id.String()}
}

// LabelsForStep returns a labels map containing the step identifier.
// The value is placed under LabelJobID for downstream correlation.
// When id is empty, it returns nil.
func LabelsForStep(id StepID) map[string]string {
	if id.IsZero() {
		return nil
	}
	return map[string]string{LabelJobID: id.String()}
}

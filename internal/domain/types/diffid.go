package types

import (
	"encoding"
	"errors"

	"github.com/google/uuid"
)

// DiffID identifies a stored diff record.
// It is UUID-backed and must be a valid UUID string when decoded from text/JSON.
type DiffID string

// ErrInvalidDiffID indicates the value is not a valid UUID.
var ErrInvalidDiffID = errors.New("invalid diff id")

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

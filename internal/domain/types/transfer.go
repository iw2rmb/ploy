package types

import (
	"encoding"
	"errors"
	"regexp"
)

// Transfer-related types for the CLI/server transfer contract.
// These enforce strict typing at API boundaries to fail fast on invalid requests.

// SlotID identifies a transfer slot.
// Values must be non-empty and URL-safe (no whitespace, no / or ? characters).
type SlotID string

// ErrInvalidSlotID indicates a SlotID value is invalid.
var ErrInvalidSlotID = errors.New("invalid slot id: must be non-empty and URL-safe")

// String returns the underlying string value.
func (v SlotID) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v SlotID) IsZero() bool { return IsEmpty(string(v)) }

// Validate checks that the SlotID is non-empty and URL-safe.
// URL-safe means no whitespace and no / or ? characters.
func (v SlotID) Validate() error {
	s := Normalize(string(v))
	if s == "" {
		return ErrInvalidSlotID
	}
	for _, c := range s {
		if c == '/' || c == '?' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			return ErrInvalidSlotID
		}
	}
	return nil
}

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*SlotID)(nil)

// MarshalText implements encoding.TextMarshaler.
func (v SlotID) MarshalText() ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	return []byte(Normalize(string(v))), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *SlotID) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	slot := SlotID(s)
	if err := slot.Validate(); err != nil {
		return err
	}
	*v = slot
	return nil
}

// MarshalJSON encodes the value as a JSON string.
func (v SlotID) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *SlotID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// TransferKind represents the type of transfer operation.
// Valid values: "repo", "artifact", "log", "cache".
type TransferKind string

// Known TransferKind values.
const (
	TransferKindRepo     TransferKind = "repo"
	TransferKindArtifact TransferKind = "artifact"
	TransferKindLog      TransferKind = "log"
	TransferKindCache    TransferKind = "cache"
)

// ErrInvalidTransferKind indicates an invalid transfer kind value.
var ErrInvalidTransferKind = errors.New("invalid transfer kind")

// String returns the underlying string value.
func (v TransferKind) String() string { return string(v) }

// IsZero reports whether the value is empty.
func (v TransferKind) IsZero() bool { return v == "" }

// Validate checks that the TransferKind is a known value.
func (v TransferKind) Validate() error {
	switch v {
	case TransferKindRepo, TransferKindArtifact, TransferKindLog, TransferKindCache:
		return nil
	default:
		return ErrInvalidTransferKind
	}
}

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*TransferKind)(nil)

// MarshalText implements encoding.TextMarshaler.
func (v TransferKind) MarshalText() ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	return []byte(v), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *TransferKind) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	kind := TransferKind(s)
	if err := kind.Validate(); err != nil {
		return err
	}
	*v = kind
	return nil
}

// MarshalJSON encodes the value as a JSON string.
func (v TransferKind) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *TransferKind) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// TransferStage represents the stage of a transfer operation.
// Valid values: "plan", "apply".
type TransferStage string

// Known TransferStage values.
const (
	TransferStagePlan  TransferStage = "plan"
	TransferStageApply TransferStage = "apply"
)

// ErrInvalidTransferStage indicates an invalid transfer stage value.
var ErrInvalidTransferStage = errors.New("invalid transfer stage")

// String returns the underlying string value.
func (v TransferStage) String() string { return string(v) }

// IsZero reports whether the value is empty.
func (v TransferStage) IsZero() bool { return v == "" }

// Validate checks that the TransferStage is a known value.
func (v TransferStage) Validate() error {
	switch v {
	case TransferStagePlan, TransferStageApply:
		return nil
	case "":
		// Empty is valid (optional field).
		return nil
	default:
		return ErrInvalidTransferStage
	}
}

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*TransferStage)(nil)

// MarshalText implements encoding.TextMarshaler.
func (v TransferStage) MarshalText() ([]byte, error) {
	if v == "" {
		return []byte{}, nil
	}
	if err := v.Validate(); err != nil {
		return nil, err
	}
	return []byte(v), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *TransferStage) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if s == "" {
		*v = ""
		return nil
	}
	stage := TransferStage(s)
	if err := stage.Validate(); err != nil {
		return err
	}
	*v = stage
	return nil
}

// MarshalJSON encodes the value as a JSON string.
func (v TransferStage) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *TransferStage) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// Digest represents a content digest in the form "sha256:<64-hex>".
// It trims surrounding spaces on decode and marshals as a JSON string.
// Validation enforces the lowercase "sha256:" prefix and a 64-character
// lowercase hexadecimal payload.
type Digest string

// digestPattern matches sha256:<64 lowercase hex chars>.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// String returns the underlying string value.
func (v Digest) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v Digest) IsZero() bool { return IsEmpty(string(v)) }

// Validate checks that the Digest is in valid sha256:<hex> format.
// Empty values are valid (digest may be optional in some contexts).
func (v Digest) Validate() error {
	s := Normalize(string(v))
	if s == "" {
		// Empty is valid (optional field).
		return nil
	}
	if !digestPattern.MatchString(s) {
		return ErrInvalidDigest
	}
	return nil
}

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*Digest)(nil)

// MarshalText implements encoding.TextMarshaler.
func (v Digest) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if s == "" {
		return []byte{}, nil
	}
	if !digestPattern.MatchString(s) {
		return nil, ErrInvalidDigest
	}
	return []byte(s), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *Digest) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if s == "" {
		*v = ""
		return nil
	}
	if !digestPattern.MatchString(s) {
		return ErrInvalidDigest
	}
	*v = Digest(s)
	return nil
}

// MarshalJSON encodes the value as a JSON string.
func (v Digest) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *Digest) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

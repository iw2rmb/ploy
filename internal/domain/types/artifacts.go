package types

import (
	"encoding"
	"errors"
	"regexp"
)

// CID is a content identifier for immutable artifact content.
//
// CID values are treated as opaque non-empty strings. They trim
// surrounding spaces on decode and marshal to/from JSON strings.
type CID string

// Sha256Digest is a content digest in the form "sha256:<64-hex>".
//
// It trims surrounding spaces on decode and marshals as a JSON string.
// Validation enforces the lowercase "sha256:" prefix and a 64-character
// lowercase hexadecimal payload.
type Sha256Digest string

// ErrInvalidDigest indicates the digest format is invalid.
var ErrInvalidDigest = errors.New("invalid digest")

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*CID)(nil)

// MarshalText implements encoding.TextMarshaler.
func (v CID) MarshalText() ([]byte, error) { return marshalIDText(v) }

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *CID) UnmarshalText(b []byte) error { return unmarshalIDText(v, b) }

// MarshalJSON encodes the value as a JSON string.
func (v CID) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *CID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// Validate implements Validatable.
func (v CID) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	return nil
}

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*Sha256Digest)(nil)

var sha256Pattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// MarshalText implements encoding.TextMarshaler.
func (v Sha256Digest) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	if !sha256Pattern.MatchString(s) {
		return nil, ErrInvalidDigest
	}
	return []byte(s), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *Sha256Digest) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	if !sha256Pattern.MatchString(s) {
		return ErrInvalidDigest
	}
	*v = Sha256Digest(s)
	return nil
}

// MarshalJSON encodes the value as a JSON string.
func (v Sha256Digest) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(v) }

// UnmarshalJSON decodes the value from a JSON string.
func (v *Sha256Digest) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }

// Validate implements Validatable.
func (v Sha256Digest) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	if !sha256Pattern.MatchString(string(v)) {
		return ErrInvalidDigest
	}
	return nil
}

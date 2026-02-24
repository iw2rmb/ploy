package types

import (
	"encoding"
	"encoding/json"
	"errors"
	"strings"
)

// Validatable is implemented by value types that can validate themselves.
type Validatable interface {
	Validate() error
}

// ErrEmpty indicates an empty or whitespace-only value.
var ErrEmpty = errors.New("empty")

// ErrInvalidMigRef indicates a MigRef value contains invalid characters.
var ErrInvalidMigRef = errors.New("invalid mig ref: contains invalid characters")

// Normalize trims surrounding whitespace from s.
func Normalize(s string) string { return strings.TrimSpace(s) }

// IsEmpty reports whether s is empty after Normalize.
func IsEmpty(s string) bool { return Normalize(s) == "" }

// MarshalJSONFromText marshals a TextMarshaler value as a JSON string.
// Intended for use by domain types that represent string values.
func MarshalJSONFromText(v encoding.TextMarshaler) ([]byte, error) {
	b, err := v.MarshalText()
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(b))
}

// UnmarshalJSONToText unmarshals a JSON string into a TextUnmarshaler.
// It rejects non-string JSON values.
func UnmarshalJSONToText(data []byte, v encoding.TextUnmarshaler) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return v.UnmarshalText([]byte(s))
}

// Strings converts a slice of string-like domain values to a slice of strings.
func Strings[T ~string](in []T) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

// StringPtr returns a pointer to the underlying string, or nil for an empty value.
func StringPtr[T ~string](v T) *string {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil
	}
	return &s
}

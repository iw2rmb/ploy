package types

import (
	"encoding"
	"errors"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a thin wrapper over time.Duration that marshals to/from
// human-readable duration strings (e.g., "1h2m3s").
//
// JSON uses a string representation, and decoding accepts strings parsed via
// time.ParseDuration with surrounding whitespace trimmed. YAML uses the same
// string form via custom marshalers.
type Duration time.Duration

// ErrInvalidDuration indicates the supplied duration string could not be parsed.
var ErrInvalidDuration = errors.New("invalid duration")

// String returns the canonical string form.
func (d Duration) String() string { return time.Duration(d).String() }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*Duration)(nil)

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) {
	// time.Duration.String() always returns a non-empty representation.
	return []byte(time.Duration(d).String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return ErrInvalidDuration
	}
	*d = Duration(v)
	return nil
}

// MarshalJSON encodes the value as a JSON string.
func (d Duration) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(d) }

// UnmarshalJSON decodes the value from a JSON string.
func (d *Duration) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, d) }

// MarshalYAML encodes the value as a YAML string node.
func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }

// UnmarshalYAML decodes the value from a YAML node expecting a string.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return ErrInvalidDuration
	}
	return d.UnmarshalText([]byte(n.Value))
}

// StdDuration converts a domain Duration to a time.Duration.
func StdDuration(d Duration) time.Duration { return time.Duration(d) }

// FromStdDuration converts a time.Duration to a domain Duration.
func FromStdDuration(d time.Duration) Duration { return Duration(d) }

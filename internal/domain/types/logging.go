package types

import (
	"encoding"
	"errors"
	"strings"
)

// LogLevel is a logging level enum with canonical string values
// "debug", "info", "warn", or "error".
//
// It trims surrounding spaces and lower-cases on decode. JSON uses string form.
// Validation rejects empty and unknown values.
type LogLevel string

const (
	// LogLevelDebug is the debug logging level in canonical lowercase form.
	LogLevelDebug LogLevel = "debug"
	// LogLevelInfo is the info logging level in canonical lowercase form.
	LogLevelInfo LogLevel = "info"
	// LogLevelWarn is the warn logging level in canonical lowercase form.
	LogLevelWarn LogLevel = "warn"
	// LogLevelError is the error logging level in canonical lowercase form.
	LogLevelError LogLevel = "error"
)

// ErrInvalidLogLevel indicates the log level is unknown.
var ErrInvalidLogLevel = errors.New("invalid log level")

// String returns the underlying level string.
func (l LogLevel) String() string { return string(l) }

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*LogLevel)(nil)

// MarshalText implements encoding.TextMarshaler.
func (l LogLevel) MarshalText() ([]byte, error) {
	s := strings.ToLower(Normalize(string(l)))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	if !allowedLogLevel(s) {
		return nil, ErrInvalidLogLevel
	}
	return []byte(s), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (l *LogLevel) UnmarshalText(b []byte) error {
	s := strings.ToLower(Normalize(string(b)))
	if IsEmpty(s) {
		return ErrEmpty
	}
	if !allowedLogLevel(s) {
		return ErrInvalidLogLevel
	}
	*l = LogLevel(s)
	return nil
}

// MarshalJSON encodes the value as a JSON string.
func (l LogLevel) MarshalJSON() ([]byte, error) { return MarshalJSONFromText(l) }

// UnmarshalJSON decodes the value from a JSON string.
func (l *LogLevel) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, l) }

// Validate implements Validatable.
func (l LogLevel) Validate() error {
	s := strings.ToLower(Normalize(string(l)))
	if IsEmpty(s) {
		return ErrEmpty
	}
	if !allowedLogLevel(s) {
		return ErrInvalidLogLevel
	}
	return nil
}

func allowedLogLevel(s string) bool {
	switch s {
	case string(LogLevelDebug), string(LogLevelInfo), string(LogLevelWarn), string(LogLevelError):
		return true
	default:
		return false
	}
}

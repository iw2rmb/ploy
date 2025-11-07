package types

import "time"

// Strings converts a slice of string-like domain values to a slice of strings.
//
// It is a generic wire adapter used when refactors are deferred and callers
// still expect []string. Values are copied; nil input returns nil.
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
//
// Empty is determined using Normalize/IsEmpty; this keeps optional JSON fields
// and query parameters omitted when not set.
func StringPtr[T ~string](v T) *string {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil
	}
	return &s
}

// StdDuration converts a domain Duration to a time.Duration.
func StdDuration(d Duration) time.Duration { return time.Duration(d) }

// FromStdDuration converts a time.Duration to a domain Duration.
func FromStdDuration(d time.Duration) Duration { return Duration(d) }

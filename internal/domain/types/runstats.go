package types

import "strings"

// RunStats represents the terminal statistics payload stored on a run.
//
// It is intentionally kept as a map-based type to preserve flexibility of the
// JSON schema while giving callers a distinct type and small helpers for
// common fields.
type RunStats map[string]any

// ExitCode returns the exit_code field as an int when present.
// It accepts int, int64, and float64 (from JSON decoding) representations.
func (s RunStats) ExitCode() (int, bool) {
	v, ok := s["exit_code"]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// Metadata returns a shallow copy of the metadata field interpreted as
// map[string]string. Non-string values are ignored.
func (s RunStats) Metadata() map[string]string {
	out := map[string]string{}
	raw, ok := s["metadata"]
	if !ok || raw == nil {
		return out
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	for k, v := range m {
		str, ok := v.(string)
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(str); trimmed != "" {
			out[k] = trimmed
		}
	}
	return out
}

// MRURL returns the mr_url entry from the metadata map when present.
func (s RunStats) MRURL() string {
	meta := s.Metadata()
	if meta == nil {
		return ""
	}
	return strings.TrimSpace(meta["mr_url"])
}

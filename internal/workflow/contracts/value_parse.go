package contracts

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseReleaseValue converts a release value (string, int, or float) to a string.
// This normalizes map-backed JSON/YAML values so all parser paths share the same
// release coercion semantics.
func ParseReleaseValue(v any, field string) (string, error) {
	switch r := v.(type) {
	case string:
		return strings.TrimSpace(r), nil
	case int:
		return fmt.Sprintf("%d", r), nil
	case int64:
		return fmt.Sprintf("%d", r), nil
	case float64:
		if r == float64(int64(r)) {
			return fmt.Sprintf("%d", int64(r)), nil
		}
		return fmt.Sprintf("%g", r), nil
	default:
		return "", fmt.Errorf("%s: expected string or number, got %T", field, v)
	}
}

// unmarshalReleaseJSON converts a json.RawMessage release value (string or number)
// to a string. It unmarshals into any and delegates to ParseReleaseValue so that
// both paths share the same coercion semantics.
func unmarshalReleaseJSON(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("release: invalid JSON: %w", err)
	}
	return ParseReleaseValue(v, "release")
}

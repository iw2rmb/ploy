package contracts

import (
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

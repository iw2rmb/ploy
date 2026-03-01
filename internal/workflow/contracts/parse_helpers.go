// parse_helpers.go provides reusable type assertion helpers for JSON/YAML parsing.
//
// These helpers standardize error messages and reduce boilerplate when parsing
// polymorphic map[string]any structures from JSON or YAML input.
package contracts

import (
	"fmt"
)

// expectString asserts v is a string and returns it, or an error with field context.
func expectString(v any, field string) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s: expected string, got %T", field, v)
	}
	return s, nil
}

// expectBool asserts v is a bool and returns it, or an error with field context.
func expectBool(v any, field string) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%s: expected bool, got %T", field, v)
	}
	return b, nil
}

// expectMap asserts v is a map[string]any and returns it, or an error with field context.
func expectMap(v any, field string) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected object, got %T", field, v)
	}
	return m, nil
}

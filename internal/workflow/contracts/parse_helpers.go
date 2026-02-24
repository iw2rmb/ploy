// parse_helpers.go provides reusable type assertion helpers for JSON/YAML parsing.
//
// These helpers standardize error messages and reduce boilerplate when parsing
// polymorphic map[string]any structures from JSON or YAML input.
package contracts

import (
	"fmt"
)

// modLikeFields holds the common fields shared between ModStep, HealingSpec, and RouterSpec.
type modLikeFields struct {
	Image           JobImage
	Command         CommandSpec
	Env             map[string]string
	RetainContainer bool
}

// parseModLikeFields parses the common fields shared by ModStep, HealingSpec, and RouterSpec.
func parseModLikeFields(raw map[string]any, prefix string) (modLikeFields, error) {
	var f modLikeFields

	// Parse image.
	if v, ok := raw["image"]; ok && v != nil {
		img, err := ParseJobImage(v)
		if err != nil {
			return f, fmt.Errorf("%s.image: %w", prefix, err)
		}
		f.Image = img
	}

	// Parse command.
	if v, ok := raw["command"]; ok && v != nil {
		cmd, err := parseCommandSpec(v)
		if err != nil {
			return f, fmt.Errorf("%s.command: %w", prefix, err)
		}
		f.Command = cmd
	}

	// Parse env.
	if v, ok := raw["env"]; ok && v != nil {
		env, err := parseEnvMap(v, prefix+".env")
		if err != nil {
			return f, err
		}
		f.Env = env
	}

	// Parse retain_container.
	if v, ok := raw["retain_container"]; ok && v != nil {
		b, err := expectBool(v, prefix+".retain_container")
		if err != nil {
			return f, err
		}
		f.RetainContainer = b
	}

	return f, nil
}

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

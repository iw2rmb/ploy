// command_spec.go provides polymorphic command handling for mig specs.
//
// CommandSpec represents container commands that can be specified in two forms:
//   - Shell string: Executed via /bin/sh -c (e.g., "echo hello && ls -la")
//   - Exec array: Executed directly without shell wrapper (e.g., ["/bin/sh", "-c", "echo"])
//
// Both forms are first-class citizens of the mig spec schema, enabling:
//   - Simple commands using a single shell string for convenience.
//   - Complex commands using exec arrays for precise control over arguments.
//
// The type implements JSON and YAML marshaling/unmarshaling to support both
// forms transparently in spec files.
package contracts

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// CommandSpec represents a container command as either a shell string or exec array.
// This type encapsulates the polymorphic command representation in mig specs.
//
// JSON/YAML Examples:
//
//	# Shell string form (executed via /bin/sh -c):
//	command: "echo hello && ls -la"
//
//	# Exec array form (executed directly):
//	command: ["/bin/sh", "-c", "echo hello"]
type CommandSpec struct {
	// Shell holds the command when specified as a single shell string.
	// When non-empty, the command is executed via ["/bin/sh", "-c", Shell].
	Shell string

	// Exec holds the command when specified as an exec array.
	// When non-nil, the command is executed directly without a shell wrapper.
	Exec []string
}

// IsEmpty returns true if no command is specified.
func (c CommandSpec) IsEmpty() bool {
	return c.Shell == "" && len(c.Exec) == 0
}

// ToSlice converts the command to a []string suitable for container execution.
// Returns nil if the command is empty.
//
// Conversion rules:
//   - Exec array: returned as-is
//   - Shell string: wrapped as ["/bin/sh", "-c", Shell]
//   - Empty: returns nil
func (c CommandSpec) ToSlice() []string {
	if len(c.Exec) > 0 {
		return c.Exec
	}
	if c.Shell != "" {
		return []string{"/bin/sh", "-c", c.Shell}
	}
	return nil
}

// ParseCommandSpec parses a command value from map-backed JSON/YAML input.
// Supported forms:
//   - string: shell command
//   - []string: exec command
//   - []any: exec command elements must all be strings
func ParseCommandSpec(v any) (CommandSpec, error) {
	switch cmd := v.(type) {
	case string:
		return CommandSpec{Shell: strings.TrimSpace(cmd)}, nil
	case []any:
		exec := make([]string, 0, len(cmd))
		for _, elem := range cmd {
			s, ok := elem.(string)
			if !ok {
				return CommandSpec{}, fmt.Errorf("expected string array element, got %T", elem)
			}
			exec = append(exec, s)
		}
		return CommandSpec{Exec: exec}, nil
	case []string:
		return CommandSpec{Exec: cmd}, nil
	default:
		return CommandSpec{}, fmt.Errorf("expected string or array, got %T", v)
	}
}

// MarshalJSON implements json.Marshaler for CommandSpec.
// Serializes as a string when Shell is set, or as an array when Exec is set.
func (c CommandSpec) MarshalJSON() ([]byte, error) {
	if len(c.Exec) > 0 {
		return json.Marshal(c.Exec)
	}
	if c.Shell != "" {
		return json.Marshal(c.Shell)
	}
	// Empty command serializes as null/omitted (via omitempty on parent).
	return json.Marshal(nil)
}

// UnmarshalJSON implements json.Unmarshaler for CommandSpec.
// Accepts both string and array forms from JSON.
func (c *CommandSpec) UnmarshalJSON(data []byte) error {
	// Handle null
	if string(data) == "null" {
		return nil
	}

	// Try string first (shell form).
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.Shell = strings.TrimSpace(s)
		return nil
	}

	// Try array (exec form).
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		c.Exec = arr
		return nil
	}

	return fmt.Errorf("command: expected string or array, got %s", string(data))
}

// MarshalYAML implements yaml.Marshaler for CommandSpec.
func (c CommandSpec) MarshalYAML() (interface{}, error) {
	if len(c.Exec) > 0 {
		return c.Exec, nil
	}
	if c.Shell != "" {
		return c.Shell, nil
	}
	return nil, nil
}

// UnmarshalYAML implements yaml.Unmarshaler for CommandSpec.
func (c *CommandSpec) UnmarshalYAML(node *yaml.Node) error {
	// Handle scalar (string form).
	if node.Kind == yaml.ScalarNode {
		c.Shell = strings.TrimSpace(node.Value)
		return nil
	}

	// Handle sequence (exec array form).
	if node.Kind == yaml.SequenceNode {
		var arr []string
		if err := node.Decode(&arr); err != nil {
			return fmt.Errorf("command array: %w", err)
		}
		c.Exec = arr
		return nil
	}

	return fmt.Errorf("command: expected string or array, got %s", node.Tag)
}

// migs_spec_parse.go provides JSON parsing for Mig specifications.
//
// Usage:
//
//	spec, err := contracts.ParseMigSpecJSON(jsonBytes)
//	if err != nil {
//	    return err // structured validation error with field paths
//	}
//
// YAML files are accepted at the CLI boundary by loading into map[string]any,
// marshaling to JSON, and validating via ParseMigSpecJSON.
package contracts

import (
	"encoding/json"
	"fmt"
)

// ParseMigSpecJSON parses a mig specification from JSON bytes.
// Returns a validated MigSpec or an error for invalid/malformed input.
func ParseMigSpecJSON(data []byte) (*MigSpec, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("steps: required")
	}

	// Validate raw contract shape through embedded MIG schema before typed unmarshaling.
	if err := ValidateMigSpecJSON(data); err != nil {
		return nil, err
	}

	// Unmarshal into typed struct.
	var spec MigSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse migs spec json: %w", err)
	}

	if err := spec.Validate(); err != nil {
		return nil, err
	}

	return &spec, nil
}

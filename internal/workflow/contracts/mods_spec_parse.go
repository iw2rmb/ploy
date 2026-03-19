// mods_spec_parse.go provides JSON parsing for Mods specifications.
//
// Usage:
//
//	spec, err := contracts.ParseModsSpecJSON(jsonBytes)
//	if err != nil {
//	    return err // structured validation error with field paths
//	}
//
// YAML files are accepted at the CLI boundary by loading into map[string]any,
// marshaling to JSON, and validating via ParseModsSpecJSON.
package contracts

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseModsSpecJSON parses a Mods specification from JSON bytes.
// Returns a validated ModsSpec or an error for invalid/malformed input.
func ParseModsSpecJSON(data []byte) (*ModsSpec, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("steps: required")
	}

	// Validate raw contract shape through embedded MIG schema before typed unmarshaling.
	if err := validateMigSpecSchema(data); err != nil {
		return nil, err
	}

	// Unmarshal into typed struct.
	var spec ModsSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse migs spec json: %w", err)
	}

	// Normalize defaults.
	if strings.TrimSpace(spec.GitLabPAT) != "" && strings.TrimSpace(spec.GitLabDomain) == "" {
		spec.GitLabDomain = "gitlab.com"
	}
	normalizeHealingDefaults(&spec)

	if err := spec.Validate(); err != nil {
		return nil, err
	}

	return &spec, nil
}

// normalizeHealingDefaults sets default values for healing action specs
// that cannot be expressed via JSON struct tags (e.g., Retries defaults to 1).
func normalizeHealingDefaults(spec *ModsSpec) {
	if spec.BuildGate == nil || spec.BuildGate.Healing == nil {
		return
	}
	for kind, action := range spec.BuildGate.Healing.ByErrorKind {
		if action.Retries == 0 {
			action.Retries = 1
			spec.BuildGate.Healing.ByErrorKind[kind] = action
		}
	}
}

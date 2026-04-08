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
	"strings"
)

// ParseMigSpecJSON parses a mig specification from JSON bytes.
// Returns a validated MigSpec or an error for invalid/malformed input.
func ParseMigSpecJSON(data []byte) (*MigSpec, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("steps: required")
	}

	// Validate raw contract shape through embedded MIG schema before typed unmarshaling.
	if err := validateMigSpecSchema(data); err != nil {
		return nil, err
	}

	// Unmarshal into typed struct.
	var spec MigSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse migs spec json: %w", err)
	}

	// Normalize defaults.
	if strings.TrimSpace(spec.GitLabPAT) != "" && strings.TrimSpace(spec.GitLabDomain) == "" {
		spec.GitLabDomain = "gitlab.com"
	}
	normalizeHealingDefaults(&spec)
	normalizeHooks(&spec)

	if err := spec.Validate(); err != nil {
		return nil, err
	}

	return &spec, nil
}

// normalizeHooks trims whitespace from hook source entries.
func normalizeHooks(spec *MigSpec) {
	for i := range spec.Hooks {
		spec.Hooks[i] = strings.TrimSpace(spec.Hooks[i])
	}
}

// normalizeHealingDefaults sets default values for the heal spec
// that cannot be expressed via JSON struct tags (e.g., Retries defaults to 1).
func normalizeHealingDefaults(spec *MigSpec) {
	if spec.BuildGate == nil || spec.BuildGate.Heal == nil {
		return
	}
	if spec.BuildGate.Heal.Retries == 0 {
		spec.BuildGate.Heal.Retries = 1
	}
}

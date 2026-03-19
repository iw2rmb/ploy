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
	"sort"
	"strings"
)

// ParseModsSpecJSON parses a Mods specification from JSON bytes.
// Returns a validated ModsSpec or an error for invalid/malformed input.
func ParseModsSpecJSON(data []byte) (*ModsSpec, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("steps: required")
	}

	// Check forbidden fields via raw map before typed unmarshaling.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse migs spec json: %w", err)
	}
	if err := checkForbiddenFields(raw); err != nil {
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

// checkForbiddenFields validates that no forbidden fields appear in the raw map.
// These are fields that have been intentionally removed from the contract or
// are derived internally and must not be user-provided.
func checkForbiddenFields(raw map[string]any) error {
	// Top-level forbidden fields.
	if _, ok := raw["mod_index"]; ok {
		return fmt.Errorf("mod_index: forbidden (derived internally from next_id; must not be provided)")
	}
	if err := checkForbiddenAmataPlacement(raw); err != nil {
		return err
	}

	// Per-step forbidden fields.
	if stepsRaw, ok := raw["steps"]; ok && stepsRaw != nil {
		if steps, ok := stepsRaw.([]any); ok {
			for i, s := range steps {
				if sm, ok := s.(map[string]any); ok {
					if _, ok := sm["retain_container"]; ok {
						return fmt.Errorf("steps[%d].retain_container: forbidden", i)
					}
				}
			}
		}
	}

	// Build gate forbidden fields.
	bgAny, ok := raw["build_gate"]
	if !ok || bgAny == nil {
		return nil
	}
	bg, ok := bgAny.(map[string]any)
	if !ok {
		return nil
	}
	if _, ok := bg["profile"]; ok {
		return fmt.Errorf("build_gate.profile: forbidden")
	}

	// Healing forbidden legacy fields.
	if healAny, ok := bg["healing"]; ok && healAny != nil {
		if heal, ok := healAny.(map[string]any); ok {
			for _, key := range []string{"retries", "image", "command", "env", "retain_container"} {
				if _, ok := heal[key]; ok {
					return fmt.Errorf("build_gate.healing.%s: forbidden (use build_gate.healing.by_error_kind.<error_kind>.%s)", key, key)
				}
			}
			// Check forbidden fields in each by_error_kind entry.
			if bekAny, ok := heal["by_error_kind"]; ok && bekAny != nil {
				if bek, ok := bekAny.(map[string]any); ok {
					for kind, entry := range bek {
						if em, ok := entry.(map[string]any); ok {
							if _, ok := em["retain_container"]; ok {
								return fmt.Errorf("build_gate.healing.by_error_kind.%s.retain_container: forbidden", kind)
							}
							// Flat spec/set are forbidden; amata config must nest under amata.{spec,set}.
							for _, key := range []string{"spec", "set"} {
								if _, ok := em[key]; ok {
									return fmt.Errorf(
										"build_gate.healing.by_error_kind.%s.%s: forbidden (use build_gate.healing.by_error_kind.%s.amata.%s)",
										kind, key, kind, key,
									)
								}
							}
						}
					}
				}
			}
		}
	}

	// Router forbidden fields.
	if routerAny, ok := bg["router"]; ok && routerAny != nil {
		if router, ok := routerAny.(map[string]any); ok {
			if _, ok := router["retain_container"]; ok {
				return fmt.Errorf("build_gate.router.retain_container: forbidden")
			}
			// Flat spec/set keys are forbidden; amata config must nest under amata.{spec,set}.
			for _, key := range []string{"spec", "set"} {
				if _, ok := router[key]; ok {
					return fmt.Errorf("build_gate.router.%s: forbidden (use build_gate.router.amata.%s)", key, key)
				}
			}
		}
	}

	return nil
}

const amataPlacementForbiddenMessage = "forbidden (amata is only allowed under build_gate.router and build_gate.healing.by_error_kind.<kind>)"

func checkForbiddenAmataPlacement(raw map[string]any) error {
	return walkForbiddenAmataPlacement(raw, "")
}

func walkForbiddenAmataPlacement(node any, path string) error {
	switch n := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(n))
		for key := range n {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			nextPath := joinRawFieldPath(path, key)
			if key == "amata" && !isAllowedAmataPath(nextPath) {
				return fmt.Errorf("%s: %s", nextPath, amataPlacementForbiddenMessage)
			}
			if err := walkForbiddenAmataPlacement(n[key], nextPath); err != nil {
				return err
			}
		}
	case []any:
		for i := range n {
			nextPath := fmt.Sprintf("%s[%d]", path, i)
			if path == "" {
				nextPath = fmt.Sprintf("[%d]", i)
			}
			if err := walkForbiddenAmataPlacement(n[i], nextPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func isAllowedAmataPath(path string) bool {
	if path == "build_gate.router.amata" {
		return true
	}
	parts := strings.Split(path, ".")
	return len(parts) == 5 &&
		parts[0] == "build_gate" &&
		parts[1] == "healing" &&
		parts[2] == "by_error_kind" &&
		parts[3] != "" &&
		parts[4] == "amata"
}

func joinRawFieldPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

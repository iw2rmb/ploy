package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
)

func parseSpecObjectStrict(spec json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(spec)) == 0 {
		return nil, fmt.Errorf("spec: expected JSON object, got empty")
	}

	var m map[string]any
	if err := json.Unmarshal(spec, &m); err != nil {
		return nil, fmt.Errorf("spec: expected JSON object: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("spec: expected JSON object, got null")
	}
	return m, nil
}

// mergeGlobalEnvIntoSpec injects global environment variables into the spec's "env" map.
// Global env vars are only merged if their scope matches the job type.
// Per-run env vars (already in spec) take precedence over global env — existing keys
// are not overwritten.
//
// Parameters:
//   - spec: The job spec JSON, may contain an "env" map
//   - env: Map of global env vars from ConfigHolder (uses typed GlobalEnvScope)
//   - modType: The job's mod_type as typed enum (pre_gate, mod, post_gate, heal, re_gate, mr)
//
// Returns the modified spec with global env vars merged into the "env" field.
func mergeGlobalEnvIntoSpec(spec json.RawMessage, env map[string]GlobalEnvVar, modType domaintypes.ModType) (json.RawMessage, error) {
	// If no global env vars exist, return spec unchanged.
	if len(env) == 0 {
		return spec, nil
	}

	// Parse the spec JSON into an object map.
	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	// Extract existing env map from spec, or create empty one.
	var em map[string]any
	if v, ok := m["env"]; ok && v != nil {
		var ok2 bool
		em, ok2 = v.(map[string]any)
		if !ok2 {
			return nil, fmt.Errorf("spec.env: expected object, got %T", v)
		}
	} else {
		em = map[string]any{}
	}

	// Merge global env vars that match the job scope.
	// Per-run env vars take precedence — skip keys that already exist.
	for k, v := range env {
		// Check if this global env var's typed scope matches the job type.
		// The scope matching uses typed enums to prevent typo-class bugs.
		if !v.Scope.MatchesModType(modType) {
			continue
		}
		// Per-run env wins over global; do not overwrite existing keys.
		if _, exists := em[k]; exists {
			continue
		}
		em[k] = v.Value
	}

	// Update the spec with merged env and serialize back to JSON.
	m["env"] = em
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal merged spec: %w", err)
	}
	return json.RawMessage(b), nil
}

// mergeGitLabConfigIntoSpec merges GitLab default token and domain into the JSON spec payload.
// Only merges values if they are non-empty and not already present in the spec.
// Per-run values (already in spec) take precedence over server defaults.
func mergeGitLabConfigIntoSpec(spec json.RawMessage, cfg config.GitLabConfig) (json.RawMessage, error) {
	// If config is empty, return spec unchanged.
	if strings.TrimSpace(cfg.Token) == "" && strings.TrimSpace(cfg.Domain) == "" {
		return spec, nil
	}

	m, err := parseSpecObjectStrict(spec)
	if err != nil {
		return nil, err
	}

	// Only add server defaults if per-run overrides are not present.
	if _, hasPerRunPAT := m["gitlab_pat"]; !hasPerRunPAT && cfg.Token != "" {
		m["gitlab_pat"] = cfg.Token
	}
	if _, hasPerRunDomain := m["gitlab_domain"]; !hasPerRunDomain && cfg.Domain != "" {
		m["gitlab_domain"] = cfg.Domain
	}

	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal merged spec: %w", err)
	}
	return json.RawMessage(b), nil
}

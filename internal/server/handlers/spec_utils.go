package handlers

import (
	"encoding/json"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
)

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
func mergeGlobalEnvIntoSpec(spec json.RawMessage, env map[string]GlobalEnvVar, modType domaintypes.ModType) json.RawMessage {
	// If no global env vars exist, return spec unchanged.
	if len(env) == 0 {
		return spec
	}

	// Parse the spec JSON into a map.
	var m map[string]any
	if len(spec) > 0 && json.Valid(spec) {
		_ = json.Unmarshal(spec, &m)
	}
	if m == nil {
		m = map[string]any{}
	}

	// Extract existing env map from spec, or create empty one.
	em, _ := m["env"].(map[string]any)
	if em == nil {
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
	b, _ := json.Marshal(m)
	return json.RawMessage(b)
}

// mergeGitLabConfigIntoSpec merges GitLab default token and domain into the JSON spec payload.
// Only merges values if they are non-empty and not already present in the spec.
// Per-run values (already in spec) take precedence over server defaults.
func mergeGitLabConfigIntoSpec(spec json.RawMessage, cfg config.GitLabConfig) json.RawMessage {
	// If config is empty, return spec unchanged.
	if strings.TrimSpace(cfg.Token) == "" && strings.TrimSpace(cfg.Domain) == "" {
		return spec
	}

	var m map[string]any
	if len(spec) > 0 && json.Valid(spec) {
		_ = json.Unmarshal(spec, &m)
	}
	if m == nil {
		m = map[string]any{}
	}

	// Only add server defaults if per-run overrides are not present.
	if _, hasPerRunPAT := m["gitlab_pat"]; !hasPerRunPAT && cfg.Token != "" {
		m["gitlab_pat"] = cfg.Token
	}
	if _, hasPerRunDomain := m["gitlab_domain"]; !hasPerRunDomain && cfg.Domain != "" {
		m["gitlab_domain"] = cfg.Domain
	}

	b, _ := json.Marshal(m)
	return json.RawMessage(b)
}

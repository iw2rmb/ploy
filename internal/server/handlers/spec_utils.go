package handlers

import (
	"encoding/json"
	"strings"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// mergeStageIDIntoSpec merges stage_id into the JSON spec payload.
func mergeStageIDIntoSpec(spec json.RawMessage, stageID string) json.RawMessage {
	if strings.TrimSpace(stageID) == "" {
		return spec
	}
	var m map[string]any
	if len(spec) > 0 && json.Valid(spec) {
		_ = json.Unmarshal(spec, &m)
	}
	if m == nil {
		m = map[string]any{}
	}
	m["stage_id"] = stageID
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

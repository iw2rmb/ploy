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

func marshalSpecObject(m map[string]any) (json.RawMessage, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal merged spec: %w", err)
	}
	return json.RawMessage(b), nil
}

func applyJobIDMutator(m map[string]any, jobID domaintypes.JobID) error {
	m["job_id"] = jobID.String()
	return nil
}

func applyGlobalEnvMutator(m map[string]any, env map[string]GlobalEnvVar, jobType domaintypes.JobType) error {
	if len(env) == 0 {
		return nil
	}

	var em map[string]any
	if v, ok := m["env"]; ok && v != nil {
		var ok2 bool
		em, ok2 = v.(map[string]any)
		if !ok2 {
			return fmt.Errorf("spec.env: expected object, got %T", v)
		}
	} else {
		em = map[string]any{}
	}

	for k, v := range env {
		if !v.Target.MatchesJobType(jobType) {
			continue
		}
		if _, exists := em[k]; exists {
			continue
		}
		em[k] = v.Value
	}
	m["env"] = em
	return nil
}

func applyGitLabConfigMutator(m map[string]any, cfg config.GitLabConfig) error {
	if strings.TrimSpace(cfg.Token) == "" && strings.TrimSpace(cfg.Domain) == "" {
		return nil
	}
	if _, hasPerRunPAT := m["gitlab_pat"]; !hasPerRunPAT && cfg.Token != "" {
		m["gitlab_pat"] = cfg.Token
	}
	if _, hasPerRunDomain := m["gitlab_domain"]; !hasPerRunDomain && cfg.Domain != "" {
		m["gitlab_domain"] = cfg.Domain
	}
	return nil
}

func ensureObjectField(parent map[string]any, key string, prefix string) (map[string]any, error) {
	if v, ok := parent[key]; ok && v != nil {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.%s: expected object, got %T", prefix, key, v)
		}
		return obj, nil
	}
	obj := map[string]any{}
	parent[key] = obj
	return obj, nil
}

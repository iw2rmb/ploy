package config

import (
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/speccompiler"
)

func parseNamedSpecEnvAllowlist(raw string) (NamedSpecEnvAllowlist, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	allowlist := make(NamedSpecEnvAllowlist, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for i, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			return nil, fmt.Errorf("config: PLOY_NAMED_SPECS_ENVS_ALLOWLIST entry %d is empty", i+1)
		}
		if !speccompiler.IsEnvironmentPlaceholderName(name) {
			return nil, fmt.Errorf("config: PLOY_NAMED_SPECS_ENVS_ALLOWLIST entry %d is not an environment variable name", i+1)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("config: PLOY_NAMED_SPECS_ENVS_ALLOWLIST contains duplicate environment variable %q", name)
		}
		seen[name] = struct{}{}
		allowlist = append(allowlist, name)
	}
	return allowlist, nil
}

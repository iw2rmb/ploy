package mods

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// applyMRAuthFromConfig resolves per-run Git provider environment from mods.yaml (mr.*)
// without embedding secrets in YAML. It reads the named env vars and maps them to
// the standard GITLAB_URL/GITLAB_TOKEN variables expected by provider and git ops.
func (r *ModRunner) applyMRAuthFromConfig(ctx context.Context) {
	if r.config == nil || r.config.MR == nil {
		return
	}
	// Token
	if name := strings.TrimSpace(r.config.MR.TokenEnv); name != "" {
		val := os.Getenv(name)
		if val != "" {
			_ = os.Setenv("PLOY_GITLAB_PAT", val)
			r.emit(ctx, "mr", "mr-config", "info", fmt.Sprintf("using token_env=%s", name))
		} else {
			r.emit(ctx, "mr", "mr-config", "warn", fmt.Sprintf("token_env=%s not set", name))
		}
	}
	// Base URL
	if name := strings.TrimSpace(r.config.MR.RepoURLEnv); name != "" {
		val := os.Getenv(name)
		if val != "" {
			_ = os.Setenv("GITLAB_URL", val)
			r.emit(ctx, "mr", "mr-config", "info", fmt.Sprintf("using repo_url_env=%s", name))
		} else {
			r.emit(ctx, "mr", "mr-config", "warn", fmt.Sprintf("repo_url_env=%s not set", name))
		}
	}
}

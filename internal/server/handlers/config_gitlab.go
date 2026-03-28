package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// GlobalEnvVar represents a single global environment variable with its metadata.
// Used by ConfigHolder to track global env entries in memory.
// The Scope field uses a typed enum (GlobalEnvScope) to prevent typo-class bugs
// in scope routing logic.
type GlobalEnvVar struct {
	Value  string                     `json:"value"`
	Scope  domaintypes.GlobalEnvScope `json:"scope"`
	Secret bool                       `json:"secret"`
}

// ConfigHolder provides thread-safe access to runtime configuration, including
// GitLab settings and global environment variables.
type ConfigHolder struct {
	mu        sync.RWMutex
	gitlab    config.GitLabConfig
	globalEnv map[string]GlobalEnvVar
}

// NewConfigHolder creates a new config holder with initial GitLab config and
// an optional map of global environment variables. The globalEnv map is copied
// to ensure the caller cannot mutate internal state.
func NewConfigHolder(gitlab config.GitLabConfig, globalEnv map[string]GlobalEnvVar) *ConfigHolder {
	// Copy the globalEnv map to avoid external mutation.
	envCopy := make(map[string]GlobalEnvVar, len(globalEnv))
	for k, v := range globalEnv {
		envCopy[k] = v
	}
	return &ConfigHolder{
		gitlab:    gitlab,
		globalEnv: envCopy,
	}
}

// GetGitLab returns the current GitLab configuration.
func (h *ConfigHolder) GetGitLab() config.GitLabConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.gitlab
}

// SetGitLab updates the GitLab configuration.
func (h *ConfigHolder) SetGitLab(cfg config.GitLabConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.gitlab = cfg
}

// GetGlobalEnv returns a copy of all global environment variables.
// The returned map is safe to mutate without affecting the holder's state.
func (h *ConfigHolder) GetGlobalEnv() map[string]GlobalEnvVar {
	h.mu.RLock()
	defer h.mu.RUnlock()
	// Return a copy to prevent external mutation.
	envCopy := make(map[string]GlobalEnvVar, len(h.globalEnv))
	for k, v := range h.globalEnv {
		envCopy[k] = v
	}
	return envCopy
}

// GetGlobalEnvVar retrieves a single global env var by key.
// Returns the variable and true if found, or a zero value and false if not.
func (h *ConfigHolder) GetGlobalEnvVar(key string) (GlobalEnvVar, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	v, ok := h.globalEnv[key]
	return v, ok
}

// SetGlobalEnvVar sets or updates a global environment variable in memory.
// Persistence to the store is the caller's responsibility.
func (h *ConfigHolder) SetGlobalEnvVar(key string, v GlobalEnvVar) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.globalEnv == nil {
		h.globalEnv = make(map[string]GlobalEnvVar)
	}
	h.globalEnv[key] = v
}

// DeleteGlobalEnvVar removes a global environment variable by key.
// No-op if the key does not exist. Persistence is the caller's responsibility.
func (h *ConfigHolder) DeleteGlobalEnvVar(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.globalEnv, key)
}

// gitLabConfigResponse is the wire format for GET/PUT /v1/config/gitlab.
type gitLabConfigResponse struct {
	Domain string `json:"domain"`
	Token  string `json:"token"`
}

// getGitLabConfigHandler returns an HTTP handler that returns the current GitLab config.
// It requires mTLS admin role authorization (enforced by middleware).
// The token field is included in the response for admin access.
func getGitLabConfigHandler(holder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := holder.GetGitLab()

		resp := gitLabConfigResponse{
			Domain: cfg.Domain,
			Token:  cfg.Token,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("config gitlab get: encode response failed", "err", err)
		}

		slog.Info("config gitlab get: returned configuration",
			"domain", cfg.Domain,
			"has_token", cfg.Token != "",
		)
	}
}

// putGitLabConfigHandler returns an HTTP handler that updates the GitLab config.
// It requires mTLS admin role authorization (enforced by middleware).
// The configuration is stored in memory only; persistence is the caller's responsibility.
func putGitLabConfigHandler(holder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Domain string `json:"domain"`
			Token  string `json:"token"`
		}

		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Update the in-memory configuration.
		holder.SetGitLab(config.GitLabConfig{
			Domain: req.Domain,
			Token:  req.Token,
		})

		// Return the updated configuration.
		resp := gitLabConfigResponse{
			Domain: req.Domain,
			Token:  req.Token,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("config gitlab put: encode response failed", "err", err)
		}

		slog.Info("config gitlab put: configuration updated",
			"domain", req.Domain,
			"has_token", req.Token != "",
		)
	}
}

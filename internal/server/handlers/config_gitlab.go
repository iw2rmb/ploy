package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"sync"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// GlobalEnvVar represents a single global environment variable with its metadata.
// Used by ConfigHolder to track global env entries in memory.
// The Target field uses a typed enum (GlobalEnvTarget) to prevent typo-class bugs
// in target routing logic.
type GlobalEnvVar struct {
	Value  string                      `json:"value"`
	Target domaintypes.GlobalEnvTarget `json:"target"`
	Secret bool                        `json:"secret"`
}

// ConfigCAEntry represents a single global CA hash entry with its section.
type ConfigCAEntry struct {
	Hash    string `json:"hash"`
	Section string `json:"section"`
}

// ConfigHomeEntry represents a single global home mount entry with its section.
type ConfigHomeEntry struct {
	Entry   string `json:"entry"`
	Dst     string `json:"dst"`
	Section string `json:"section"`
}

// ConfigHolder provides thread-safe access to runtime configuration, including
// GitLab settings, global environment variables, and typed Hydra overlays.
// Global env is stored as key → []GlobalEnvVar to support multiple targets per key.
// Hydra overlays are stored per section (pre_gate, re_gate, post_gate, mig, heal).
// Config CA and Home entries are section-keyed and synced into hydra overlays.
type ConfigHolder struct {
	mu        sync.RWMutex
	gitlab    config.GitLabConfig
	globalEnv map[string][]GlobalEnvVar
	hydra     map[string]*HydraJobConfig
	configCA  map[string][]string // section → []hash
	configHome map[string][]ConfigHomeEntry // section → []entry
}

// NewConfigHolder creates a new config holder with initial GitLab config and
// an optional multi-target map of global environment variables.
func NewConfigHolder(gitlab config.GitLabConfig, globalEnv map[string][]GlobalEnvVar) *ConfigHolder {
	envCopy := make(map[string][]GlobalEnvVar, len(globalEnv))
	for k, entries := range globalEnv {
		cp := make([]GlobalEnvVar, len(entries))
		copy(cp, entries)
		envCopy[k] = cp
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

// GetGlobalEnvEntries retrieves all entries for a key (one per target).
// Returns nil if the key does not exist.
func (h *ConfigHolder) GetGlobalEnvEntries(key string) []GlobalEnvVar {
	h.mu.RLock()
	defer h.mu.RUnlock()
	entries := h.globalEnv[key]
	if len(entries) == 0 {
		return nil
	}
	cp := make([]GlobalEnvVar, len(entries))
	copy(cp, entries)
	return cp
}

// GetGlobalEnvVar retrieves a single global env var by key.
// Returns the first entry and true if found, or a zero value and false if not.
func (h *ConfigHolder) GetGlobalEnvVar(key string) (GlobalEnvVar, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	entries := h.globalEnv[key]
	if len(entries) == 0 {
		return GlobalEnvVar{}, false
	}
	return entries[0], true
}

// SetGlobalEnvVar sets or updates a global environment variable by key+target.
// If an entry for this key+target already exists, it is replaced.
// Persistence to the store is the caller's responsibility.
func (h *ConfigHolder) SetGlobalEnvVar(key string, v GlobalEnvVar) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.globalEnv == nil {
		h.globalEnv = make(map[string][]GlobalEnvVar)
	}
	entries := h.globalEnv[key]
	for i, e := range entries {
		if e.Target == v.Target {
			entries[i] = v
			h.globalEnv[key] = entries
			return
		}
	}
	h.globalEnv[key] = append(entries, v)
}

// DeleteGlobalEnvVar removes a global environment variable by key and target.
// No-op if the key+target does not exist. Persistence is the caller's responsibility.
func (h *ConfigHolder) DeleteGlobalEnvVar(key string, target domaintypes.GlobalEnvTarget) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries := h.globalEnv[key]
	for i, e := range entries {
		if e.Target == target {
			h.globalEnv[key] = append(entries[:i], entries[i+1:]...)
			if len(h.globalEnv[key]) == 0 {
				delete(h.globalEnv, key)
			}
			return
		}
	}
}

// GetHydraOverlays returns a deep copy of all Hydra overlays keyed by section.
func (h *ConfigHolder) GetHydraOverlays() map[string]*HydraJobConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.hydra) == 0 {
		return nil
	}
	cp := make(map[string]*HydraJobConfig, len(h.hydra))
	for k, v := range h.hydra {
		cp[k] = &HydraJobConfig{
			Envs: copyStringMap(v.Envs),
			CA:   copyStringSlice(v.CA),
			In:   copyStringSlice(v.In),
			Out:  copyStringSlice(v.Out),
			Home: copyStringSlice(v.Home),
		}
	}
	return cp
}

// SetHydraJobConfig sets the Hydra overlay for a named section.
// Section must be one of: pre_gate, re_gate, post_gate, mig, heal.
func (h *ConfigHolder) SetHydraJobConfig(section string, cfg *HydraJobConfig) error {
	if err := ValidateHydraSection(section); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hydra == nil {
		h.hydra = make(map[string]*HydraJobConfig)
	}
	if cfg == nil {
		delete(h.hydra, section)
	} else {
		h.hydra[section] = &HydraJobConfig{
			Envs: copyStringMap(cfg.Envs),
			CA:   copyStringSlice(cfg.CA),
			In:   copyStringSlice(cfg.In),
			Out:  copyStringSlice(cfg.Out),
			Home: copyStringSlice(cfg.Home),
		}
	}
	return nil
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

// GetConfigCA returns all CA hashes for a section (sorted).
func (h *ConfigHolder) GetConfigCA(section string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	entries := h.configCA[section]
	if len(entries) == 0 {
		return nil
	}
	cp := make([]string, len(entries))
	copy(cp, entries)
	return cp
}

// GetConfigCAAll returns a copy of all CA entries keyed by section.
func (h *ConfigHolder) GetConfigCAAll() map[string][]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.configCA) == 0 {
		return nil
	}
	cp := make(map[string][]string, len(h.configCA))
	for k, v := range h.configCA {
		s := make([]string, len(v))
		copy(s, v)
		cp[k] = s
	}
	return cp
}

// SetConfigCA replaces the CA hash set for a section and syncs into hydra overlays.
func (h *ConfigHolder) SetConfigCA(section string, hashes []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.configCA == nil {
		h.configCA = make(map[string][]string)
	}
	if len(hashes) == 0 {
		delete(h.configCA, section)
	} else {
		cp := make([]string, len(hashes))
		copy(cp, hashes)
		h.configCA[section] = cp
	}
	h.syncHydraCALocked(section)
}

// AddConfigCA adds a single CA hash to a section (dedup, sort, then sync).
func (h *ConfigHolder) AddConfigCA(section, hash string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.configCA == nil {
		h.configCA = make(map[string][]string)
	}
	for _, existing := range h.configCA[section] {
		if existing == hash {
			return // already present
		}
	}
	h.configCA[section] = append(h.configCA[section], hash)
	sort.Strings(h.configCA[section])
	h.syncHydraCALocked(section)
}

// DeleteConfigCA removes a single CA hash from a section.
func (h *ConfigHolder) DeleteConfigCA(section, hash string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries := h.configCA[section]
	for i, e := range entries {
		if e == hash {
			h.configCA[section] = append(entries[:i], entries[i+1:]...)
			if len(h.configCA[section]) == 0 {
				delete(h.configCA, section)
			}
			break
		}
	}
	h.syncHydraCALocked(section)
}

// syncHydraCALocked updates the hydra overlay CA field for a section.
// Must be called with h.mu held.
func (h *ConfigHolder) syncHydraCALocked(section string) {
	if h.hydra == nil {
		h.hydra = make(map[string]*HydraJobConfig)
	}
	cfg := h.hydra[section]
	if cfg == nil {
		cfg = &HydraJobConfig{}
		h.hydra[section] = cfg
	}
	cfg.CA = copyStringSlice(h.configCA[section])
}

// GetConfigHome returns all home entries for a section (sorted by dst).
func (h *ConfigHolder) GetConfigHome(section string) []ConfigHomeEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	entries := h.configHome[section]
	if len(entries) == 0 {
		return nil
	}
	cp := make([]ConfigHomeEntry, len(entries))
	copy(cp, entries)
	return cp
}

// GetConfigHomeAll returns a copy of all home entries keyed by section.
func (h *ConfigHolder) GetConfigHomeAll() map[string][]ConfigHomeEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.configHome) == 0 {
		return nil
	}
	cp := make(map[string][]ConfigHomeEntry, len(h.configHome))
	for k, v := range h.configHome {
		s := make([]ConfigHomeEntry, len(v))
		copy(s, v)
		cp[k] = s
	}
	return cp
}

// SetConfigHome replaces the home entry set for a section and syncs into hydra overlays.
func (h *ConfigHolder) SetConfigHome(section string, entries []ConfigHomeEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.configHome == nil {
		h.configHome = make(map[string][]ConfigHomeEntry)
	}
	if len(entries) == 0 {
		delete(h.configHome, section)
	} else {
		cp := make([]ConfigHomeEntry, len(entries))
		copy(cp, entries)
		h.configHome[section] = cp
	}
	h.syncHydraHomeLocked(section)
}

// AddConfigHome adds or replaces a home entry by destination in a section (dedup by dst, sort by dst).
func (h *ConfigHolder) AddConfigHome(section string, entry ConfigHomeEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.configHome == nil {
		h.configHome = make(map[string][]ConfigHomeEntry)
	}
	entries := h.configHome[section]
	for i, e := range entries {
		if e.Dst == entry.Dst {
			entries[i] = entry
			h.configHome[section] = entries
			h.syncHydraHomeLocked(section)
			return
		}
	}
	h.configHome[section] = append(entries, entry)
	sort.Slice(h.configHome[section], func(i, j int) bool {
		return h.configHome[section][i].Dst < h.configHome[section][j].Dst
	})
	h.syncHydraHomeLocked(section)
}

// DeleteConfigHome removes a home entry by destination from a section.
func (h *ConfigHolder) DeleteConfigHome(section, dst string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries := h.configHome[section]
	for i, e := range entries {
		if e.Dst == dst {
			h.configHome[section] = append(entries[:i], entries[i+1:]...)
			if len(h.configHome[section]) == 0 {
				delete(h.configHome, section)
			}
			break
		}
	}
	h.syncHydraHomeLocked(section)
}

// syncHydraHomeLocked updates the hydra overlay Home field for a section.
// Must be called with h.mu held.
func (h *ConfigHolder) syncHydraHomeLocked(section string) {
	if h.hydra == nil {
		h.hydra = make(map[string]*HydraJobConfig)
	}
	cfg := h.hydra[section]
	if cfg == nil {
		cfg = &HydraJobConfig{}
		h.hydra[section] = cfg
	}
	entries := h.configHome[section]
	home := make([]string, len(entries))
	for i, e := range entries {
		home[i] = e.Entry
	}
	cfg.Home = home
}

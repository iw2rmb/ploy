package handlers

import (
	"sort"
	"sync"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
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

// ConfigInEntry represents a single global in mount entry with its section.
type ConfigInEntry struct {
	Entry   string `json:"entry"`
	Dst     string `json:"dst"`
	Section string `json:"section"`
}

// ConfigHolder provides thread-safe access to runtime configuration, including
// global environment variables and typed Hydra overlays.
// Global env is stored as key → []GlobalEnvVar to support multiple targets per key.
// Hydra overlays are stored per section (pre_gate, post_gate, mig).
// Config In entries are section-keyed and synced into hydra overlays.
type ConfigHolder struct {
	mu        sync.RWMutex
	globalEnv map[string][]GlobalEnvVar
	hydra     map[string]*HydraJobConfig
	configIn  map[string][]ConfigInEntry // section → []entry
	bundleMap map[string]string          // shortHash → bundleID (content-addressed, global)
}

// NewConfigHolder creates a new config holder with an optional multi-target map
// of global environment variables.
func NewConfigHolder(globalEnv map[string][]GlobalEnvVar) *ConfigHolder {
	envCopy := make(map[string][]GlobalEnvVar, len(globalEnv))
	for k, entries := range globalEnv {
		cp := make([]GlobalEnvVar, len(entries))
		copy(cp, entries)
		envCopy[k] = cp
	}
	return &ConfigHolder{
		globalEnv: envCopy,
	}
}

func copySectionSlice[T any](m map[string][]T, section string) []T {
	entries := m[section]
	if len(entries) == 0 {
		return nil
	}
	cp := make([]T, len(entries))
	copy(cp, entries)
	return cp
}

func setSectionSlice[T any](m map[string][]T, section string, entries []T) map[string][]T {
	if m == nil {
		m = make(map[string][]T)
	}
	if len(entries) == 0 {
		delete(m, section)
		return m
	}
	cp := make([]T, len(entries))
	copy(cp, entries)
	m[section] = cp
	return m
}

func upsertSectionBy[T any](m map[string][]T, section string, entry T, match func(a, b T) bool, less func(a, b T) bool) map[string][]T {
	if m == nil {
		m = make(map[string][]T)
	}
	entries := m[section]
	for i := range entries {
		if match(entries[i], entry) {
			entries[i] = entry
			m[section] = entries
			return m
		}
	}
	entries = append(entries, entry)
	if less != nil {
		sort.Slice(entries, func(i, j int) bool { return less(entries[i], entries[j]) })
	}
	m[section] = entries
	return m
}

func deleteSectionBy[T any](m map[string][]T, section string, match func(T) bool) map[string][]T {
	entries := m[section]
	for i := range entries {
		if !match(entries[i]) {
			continue
		}
		entries = append(entries[:i], entries[i+1:]...)
		if len(entries) == 0 {
			delete(m, section)
		} else {
			m[section] = entries
		}
		return m
	}
	return m
}

func (h *ConfigHolder) ensureHydraSectionLocked(section string) *HydraJobConfig {
	if h.hydra == nil {
		h.hydra = make(map[string]*HydraJobConfig)
	}
	cfg := h.hydra[section]
	if cfg == nil {
		cfg = &HydraJobConfig{}
		h.hydra[section] = cfg
	}
	return cfg
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
			In:   copyStringSlice(v.In),
			Out:  copyStringSlice(v.Out),
		}
	}
	return cp
}

// SetConfigIn replaces the in entry set for a section and syncs into hydra overlays.
func (h *ConfigHolder) SetConfigIn(section string, entries []ConfigInEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.configIn = setSectionSlice(h.configIn, section, entries)
	h.syncHydraInLocked(section)
}

// AddConfigIn adds or replaces an in entry by destination in a section (dedup by dst, sort by dst).
func (h *ConfigHolder) AddConfigIn(section string, entry ConfigInEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.configIn = upsertSectionBy(
		h.configIn,
		section,
		entry,
		func(a, b ConfigInEntry) bool { return a.Dst == b.Dst },
		func(a, b ConfigInEntry) bool { return a.Dst < b.Dst },
	)
	h.syncHydraInLocked(section)
}

// DeleteConfigIn removes an in entry by destination from a section.
func (h *ConfigHolder) DeleteConfigIn(section, dst string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.configIn = deleteSectionBy(h.configIn, section, func(e ConfigInEntry) bool { return e.Dst == dst })
	h.syncHydraInLocked(section)
}

// syncHydraInLocked updates the hydra overlay In field for a section.
// Must be called with h.mu held.
func (h *ConfigHolder) syncHydraInLocked(section string) {
	cfg := h.ensureHydraSectionLocked(section)
	entries := h.configIn[section]
	in := make([]string, len(entries))
	for i, e := range entries {
		in[i] = e.Entry
	}
	cfg.In = in
}

// AddBundleMapping stores a shortHash → bundleID mapping so that the claim
// mutator can thread server-side bundle references into spec bundle_map.
func (h *ConfigHolder) AddBundleMapping(hash, bundleID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.bundleMap == nil {
		h.bundleMap = make(map[string]string)
	}
	h.bundleMap[hash] = bundleID
}

// GetBundleMap returns a copy of all shortHash → bundleID mappings.
func (h *ConfigHolder) GetBundleMap() map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.bundleMap) == 0 {
		return nil
	}
	cp := make(map[string]string, len(h.bundleMap))
	for k, v := range h.bundleMap {
		cp[k] = v
	}
	return cp
}

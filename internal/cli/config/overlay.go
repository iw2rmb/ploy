// overlay.go implements local config.yaml overlay loading and deterministic
// merge for the Hydra envs/ca/in/out/home contract.
//
// The config.yaml lives at $PLOY_CONFIG_HOME/config.yaml and supports:
//
//	defaults:
//	  server:   {envs, ca, home}
//	  node:     {envs, ca, home}
//	  job:
//	    pre_gate:  {envs, ca, in, out, home}
//	    re_gate:   {envs, ca, in, out, home}
//	    post_gate: {envs, ca, in, out, home}
//	    mig:       {envs, ca, in, out, home}
//	    heal:      {envs, ca, in, out, home}
//
// Merge precedence (lowest → highest): server defaults < local config.yaml < spec.
// Per-field merge rules:
//   - envs: key-based override by precedence
//   - ca:   append with dedup by value (digest-like strings)
//   - in/out/home: merge by destination; higher precedence replaces same destination
//
// Router containers inherit the active gate phase section.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComponentConfig holds envs/ca/home for server and node component sections.
type ComponentConfig struct {
	Envs map[string]string `yaml:"envs,omitempty" json:"envs,omitempty"`
	CA   []string          `yaml:"ca,omitempty" json:"ca,omitempty"`
	Home []string          `yaml:"home,omitempty" json:"home,omitempty"`
}

// JobConfig holds the full Hydra field set for job container sections.
type JobConfig struct {
	Envs map[string]string `yaml:"envs,omitempty" json:"envs,omitempty"`
	CA   []string          `yaml:"ca,omitempty" json:"ca,omitempty"`
	In   []string          `yaml:"in,omitempty" json:"in,omitempty"`
	Out  []string          `yaml:"out,omitempty" json:"out,omitempty"`
	Home []string          `yaml:"home,omitempty" json:"home,omitempty"`
}

// JobTargets groups the per-phase job config sections.
type JobTargets struct {
	PreGate  *JobConfig `yaml:"pre_gate,omitempty" json:"pre_gate,omitempty"`
	ReGate   *JobConfig `yaml:"re_gate,omitempty" json:"re_gate,omitempty"`
	PostGate *JobConfig `yaml:"post_gate,omitempty" json:"post_gate,omitempty"`
	Mig      *JobConfig `yaml:"mig,omitempty" json:"mig,omitempty"`
	Heal     *JobConfig `yaml:"heal,omitempty" json:"heal,omitempty"`
}

// Defaults is the top-level defaults section of config.yaml.
type Defaults struct {
	Server *ComponentConfig `yaml:"server,omitempty" json:"server,omitempty"`
	Node   *ComponentConfig `yaml:"node,omitempty" json:"node,omitempty"`
	Job    *JobTargets      `yaml:"job,omitempty" json:"job,omitempty"`
}

// Overlay represents a parsed $PLOY_CONFIG_HOME/config.yaml.
type Overlay struct {
	Defaults *Defaults `yaml:"defaults,omitempty" json:"defaults,omitempty"`
}

// LoadOverlay reads and parses $PLOY_CONFIG_HOME/config.yaml.
// Returns a zero Overlay (not an error) when the file does not exist.
func LoadOverlay() (Overlay, error) {
	base, err := configBaseDir()
	if err != nil {
		return Overlay{}, err
	}
	return LoadOverlayFrom(filepath.Join(base, "config.yaml"))
}

// LoadOverlayFrom reads and parses a config.yaml at the given path.
// Returns a zero Overlay (not an error) when the file does not exist.
func LoadOverlayFrom(path string) (Overlay, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Overlay{}, nil
		}
		return Overlay{}, fmt.Errorf("config overlay: read %s: %w", path, err)
	}
	var ov Overlay
	if err := yaml.Unmarshal(data, &ov); err != nil {
		return Overlay{}, fmt.Errorf("config overlay: parse %s: %w", path, err)
	}
	return ov, nil
}

// JobSection returns the JobConfig for the given job type string.
// Known types: "pre_gate", "re_gate", "post_gate", "mig", "heal".
// Returns nil when the overlay has no config for the given type.
func (o *Overlay) JobSection(jobType string) *JobConfig {
	if o.Defaults == nil || o.Defaults.Job == nil {
		return nil
	}
	switch jobType {
	case "pre_gate":
		return o.Defaults.Job.PreGate
	case "re_gate":
		return o.Defaults.Job.ReGate
	case "post_gate":
		return o.Defaults.Job.PostGate
	case "mig":
		return o.Defaults.Job.Mig
	case "heal":
		return o.Defaults.Job.Heal
	default:
		return nil
	}
}

// RouterSection returns the JobConfig for the active gate phase that a router
// container inherits. The caller derives the gate phase from the spec's
// build_gate configuration. Valid values: "pre_gate", "re_gate", "post_gate".
func (o *Overlay) RouterSection(gatePhase string) *JobConfig {
	return o.JobSection(gatePhase)
}

// MergeJobConfigIntoSpec applies a JobConfig overlay onto a spec container block
// (map[string]any) using deterministic merge rules:
//   - envs: key-based override (overlay wins for same key, spec wins overall)
//   - ca: append with dedup by value
//   - in/out/home: merge by destination; spec entry replaces overlay for same dst
//
// This implements the "local config < spec" precedence for a single block.
func MergeJobConfigIntoSpec(block map[string]any, cfg *JobConfig) {
	if cfg == nil {
		return
	}
	mergeEnvsIntoBlock(block, cfg.Envs)
	mergeCAIntoBlock(block, cfg.CA)
	mergeRecordsByDst(block, "in", cfg.In)
	mergeRecordsByDst(block, "out", cfg.Out)
	mergeRecordsByDst(block, "home", cfg.Home)
}

// mergeEnvsIntoBlock merges overlay envs into block["envs"]. Existing spec keys win.
func mergeEnvsIntoBlock(block map[string]any, overlay map[string]string) {
	if len(overlay) == 0 {
		return
	}
	existing := make(map[string]any)
	if raw, ok := block["envs"].(map[string]any); ok {
		for k, v := range raw {
			existing[k] = v
		}
	}
	for k, v := range overlay {
		if _, has := existing[k]; !has {
			existing[k] = v
		}
	}
	block["envs"] = existing
}

// mergeCAIntoBlock appends overlay CA entries, deduplicating by value.
func mergeCAIntoBlock(block map[string]any, overlay []string) {
	if len(overlay) == 0 {
		return
	}
	seen := make(map[string]bool)
	var merged []any

	// Existing spec CA entries first (higher precedence).
	if raw, ok := block["ca"].([]any); ok {
		for _, e := range raw {
			s, ok := e.(string)
			if !ok {
				merged = append(merged, e)
				continue
			}
			key := caDedup(s)
			if !seen[key] {
				seen[key] = true
				merged = append(merged, s)
			}
		}
	}

	// Overlay CA entries appended, deduped.
	for _, s := range overlay {
		key := caDedup(s)
		if !seen[key] {
			seen[key] = true
			merged = append(merged, s)
		}
	}

	if len(merged) > 0 {
		block["ca"] = merged
	}
}

// caDedup returns a dedup key for a CA entry. For short-hash entries use as-is;
// for file paths, hash the value to avoid path-vs-digest comparison issues.
func caDedup(s string) string {
	s = strings.TrimSpace(s)
	// If it looks like a hex hash (7-64 chars), use it directly.
	if len(s) >= 7 && len(s) <= 64 && isHex(s) {
		return s
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// mergeRecordsByDst merges overlay entries into block[field] by destination.
// Spec entries win when the destination matches.
func mergeRecordsByDst(block map[string]any, field string, overlay []string) {
	if len(overlay) == 0 {
		return
	}

	specDsts := make(map[string]bool)
	var merged []any

	if raw, ok := block[field].([]any); ok {
		for _, e := range raw {
			s, ok := e.(string)
			if !ok {
				merged = append(merged, e)
				continue
			}
			dst := extractDst(field, s)
			specDsts[dst] = true
			merged = append(merged, s)
		}
	}

	for _, s := range overlay {
		dst := extractDst(field, s)
		if !specDsts[dst] {
			specDsts[dst] = true
			merged = append(merged, s)
		}
	}

	if len(merged) > 0 {
		block[field] = merged
	}
}

// extractDst extracts the destination from an authoring or canonical entry.
// For in/out: dst is everything after the last colon (or first colon for hash:dst).
// For home: dst is the middle segment (src:dst or src:dst:ro / hash:dst or hash:dst:ro).
func extractDst(field, entry string) string {
	switch field {
	case "home":
		body := strings.TrimSuffix(entry, ":ro")
		idx := strings.LastIndex(body, ":")
		if idx >= 0 {
			return body[idx+1:]
		}
		return body
	default: // in, out
		idx := strings.LastIndex(entry, ":")
		if idx >= 0 {
			return entry[idx+1:]
		}
		return entry
	}
}

// SortedEnvKeys returns sorted keys from an envs map for deterministic output.
func SortedEnvKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

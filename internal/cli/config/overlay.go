// overlay.go implements local config.yaml overlay loading and deterministic
// merge for the Hydra envs/in/out/home contract.
//
// The config.yaml lives at $PLOY_CONFIG_HOME/config.yaml and supports:
//
//	defaults:
//	  server:   {envs, home}
//	  node:     {envs, home}
//	  job:
//	    pre_gate:  {envs, in, out, home}
//	    post_gate: {envs, in, out, home}
//	    mig:       {envs, in, out, home}
//
// Merge precedence (lowest → highest): server defaults < local config.yaml < spec.
// Per-field merge rules:
//   - envs: key-based override by precedence
//   - in/out/home: merge by destination; higher precedence replaces same destination
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComponentConfig holds envs/home for server and node component sections.
type ComponentConfig struct {
	Envs map[string]string `yaml:"envs,omitempty" json:"envs,omitempty"`
	Home []string          `yaml:"home,omitempty" json:"home,omitempty"`
}

// JobConfig holds the full Hydra field set for job container sections.
type JobConfig struct {
	Envs map[string]string `yaml:"envs,omitempty" json:"envs,omitempty"`
	In   []string          `yaml:"in,omitempty" json:"in,omitempty"`
	Out  []string          `yaml:"out,omitempty" json:"out,omitempty"`
	Home []string          `yaml:"home,omitempty" json:"home,omitempty"`
}

// JobTargets groups the per-phase job config sections.
type JobTargets struct {
	PreGate  *JobConfig `yaml:"pre_gate,omitempty" json:"pre_gate,omitempty"`
	PostGate *JobConfig `yaml:"post_gate,omitempty" json:"post_gate,omitempty"`
	Mig      *JobConfig `yaml:"mig,omitempty" json:"mig,omitempty"`
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
// Known types: "pre_gate", "post_gate", "mig".
// Returns nil when the overlay has no config for the given type.
func (o *Overlay) JobSection(jobType string) *JobConfig {
	if o.Defaults == nil || o.Defaults.Job == nil {
		return nil
	}
	switch jobType {
	case "pre_gate":
		return o.Defaults.Job.PreGate
	case "post_gate":
		return o.Defaults.Job.PostGate
	case "mig":
		return o.Defaults.Job.Mig
	default:
		return nil
	}
}

// MergeJobConfigIntoSpec applies a JobConfig overlay onto a spec container block
// (map[string]any) using deterministic merge rules:
//   - envs: key-based override (overlay wins for same key, spec wins overall)
//   - in/out/home: merge by destination; spec entry replaces overlay for same dst
//
// This implements the "local config < spec" precedence for a single block.
func MergeJobConfigIntoSpec(block map[string]any, cfg *JobConfig) {
	if cfg == nil {
		return
	}
	mergeEnvsIntoBlock(block, cfg.Envs)
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

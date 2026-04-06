package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// HydraJobConfig holds the typed Hydra overlay fields for a single job section.
// Used by the claim mutator pipeline to merge server-side configuration into
// the claim spec using per-field merge strategies.
type HydraJobConfig struct {
	Envs map[string]string
	CA   []string
	In   []string
	Out  []string
	Home []string
}

// IsEmpty reports whether all fields are empty.
func (c *HydraJobConfig) IsEmpty() bool {
	if c == nil {
		return true
	}
	return len(c.Envs) == 0 && len(c.CA) == 0 && len(c.In) == 0 && len(c.Out) == 0 && len(c.Home) == 0
}

// ValidateHydraSection delegates to contracts.ValidateHydraSection for
// backward compatibility within the handlers package.
func ValidateHydraSection(section string) error {
	return contracts.ValidateHydraSection(section)
}

// applyHydraOverlayMutator replaces the legacy env-only merge with a typed merge
// for envs, ca, in, out, and home using deterministic ordering.
//
// Section routing resolves the overlay for the job type from the hydra overlay
// map. Global env vars are folded into the overlay's envs field using
// target-aware precedence (nodes < job-target < per-run spec).
//
// For specs with a heal configuration, the heal section overlay is applied
// to build_gate.heal.
//
// Merge precedence: per-run spec values always win over overlay values.
func applyHydraOverlayMutator(m map[string]any, in claimSpecMutatorInput) error {
	section := string(in.jobType)
	overlay := assembleHydraOverlay(in.hydraOverlays, section, in.globalEnv, in.jobType)

	if err := validateOverlayCollisions(overlay, "spec"); err != nil {
		return err
	}

	// Keep legacy top-level merge for compatibility with existing consumers/tests.
	mergeHydraIntoBlock(m, overlay)
	// Ensure typed fields are injected into canonical schema locations consumed
	// by node parsing for each job phase.
	applyCanonicalHydraOverlay(m, in.jobType, overlay)

	if err := applyHealOverlay(m, in); err != nil {
		return err
	}

	return nil
}

func applyCanonicalHydraOverlay(spec map[string]any, jobType domaintypes.JobType, overlay *HydraJobConfig) {
	if spec == nil || overlay == nil {
		return
	}
	switch jobType {
	case domaintypes.JobTypeMig:
		applyOverlayToSteps(spec, overlay)
	case domaintypes.JobTypePreGate:
		mergeCABlock(ensureBuildGatePhase(spec, "pre"), overlay.CA)
	case domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
		mergeCABlock(ensureBuildGatePhase(spec, "post"), overlay.CA)
	}
}

func applyOverlayToSteps(spec map[string]any, overlay *HydraJobConfig) {
	rawSteps, ok := spec["steps"].([]any)
	if !ok || len(rawSteps) == 0 {
		return
	}
	for i := range rawSteps {
		step, ok := rawSteps[i].(map[string]any)
		if !ok || step == nil {
			continue
		}
		mergeCABlock(step, overlay.CA)
		mergeRecordsByDstBlock(step, "in", overlay.In)
		mergeRecordsByDstBlock(step, "out", overlay.Out)
		mergeRecordsByDstBlock(step, "home", overlay.Home)
	}
}

func ensureBuildGatePhase(spec map[string]any, phase string) map[string]any {
	bg, ok := spec["build_gate"].(map[string]any)
	if !ok || bg == nil {
		bg = make(map[string]any)
		spec["build_gate"] = bg
	}
	block, ok := bg[phase].(map[string]any)
	if !ok || block == nil {
		block = make(map[string]any)
		bg[phase] = block
	}
	return block
}

// assembleHydraOverlay builds the complete HydraJobConfig for a job section by
// looking up the typed overlay and folding global env vars into the envs field
// with target-aware precedence.
func assembleHydraOverlay(
	overlays map[string]*HydraJobConfig,
	section string,
	globalEnv map[string][]GlobalEnvVar,
	jobType domaintypes.JobType,
) *HydraJobConfig {
	base := &HydraJobConfig{}
	if cfg, ok := overlays[section]; ok && cfg != nil {
		base = &HydraJobConfig{
			Envs: copyStringMap(cfg.Envs),
			CA:   copyStringSlice(cfg.CA),
			In:   copyStringSlice(cfg.In),
			Out:  copyStringSlice(cfg.Out),
			Home: copyStringSlice(cfg.Home),
		}
	}

	// Two-pass global env resolution (same logic as legacy applyGlobalEnvMutator).
	// Special env keys (file-backed, migrated to typed Hydra fields) are
	// excluded — they must not be folded as raw env vars into job envs.
	globalMerged := make(map[string]string)

	// Pass 1: nodes-target (lowest priority among global env).
	for k, entries := range globalEnv {
		if IsSpecialEnvKey(k) {
			continue
		}
		for _, v := range entries {
			if v.Target == domaintypes.GlobalEnvTargetNodes {
				globalMerged[k] = v.Value
			}
		}
	}

	// Pass 2: job-target (gates or steps) overrides nodes-target.
	for k, entries := range globalEnv {
		if IsSpecialEnvKey(k) {
			continue
		}
		for _, v := range entries {
			if v.Target.MatchesJobType(jobType) {
				globalMerged[k] = v.Value
			}
		}
	}

	// Overlay envs override global env vars for the same key (both are
	// "server defaults" but typed overlay has slight priority over legacy
	// global env).
	if base.Envs == nil && len(globalMerged) > 0 {
		base.Envs = globalMerged
	} else {
		for k, v := range globalMerged {
			if _, exists := base.Envs[k]; !exists {
				base.Envs[k] = v
			}
		}
	}

	return base
}

// applyHealOverlay applies the heal section overlay to build_gate.heal when
// it exists. Returns a collision error when the heal overlay contains
// duplicate destinations.
func applyHealOverlay(m map[string]any, in claimSpecMutatorInput) error {
	bg, ok := m["build_gate"].(map[string]any)
	if !ok {
		return nil
	}
	healBlock, ok := bg["heal"].(map[string]any)
	if !ok {
		return nil
	}

	overlay := assembleHydraOverlay(in.hydraOverlays, "heal", nil, domaintypes.JobTypeHeal)
	if overlay.IsEmpty() {
		return nil
	}

	if err := validateOverlayCollisions(overlay, "build_gate.heal"); err != nil {
		return err
	}

	mergeHydraIntoBlock(healBlock, overlay)
	return nil
}

// mergeHydraIntoBlock applies all overlay fields into a spec block using
// per-field merge rules. Existing block values take precedence.
func mergeHydraIntoBlock(block map[string]any, cfg *HydraJobConfig) {
	if cfg == nil {
		return
	}
	mergeEnvsBlock(block, cfg.Envs)
	mergeCABlock(block, cfg.CA)
	mergeRecordsByDstBlock(block, "in", cfg.In)
	mergeRecordsByDstBlock(block, "out", cfg.Out)
	mergeRecordsByDstBlock(block, "home", cfg.Home)
}

// mergeEnvsBlock merges overlay envs into block["envs"]. Existing block keys
// win for the same key. Insertion uses sorted key order for determinism.
func mergeEnvsBlock(block map[string]any, overlay map[string]string) {
	if len(overlay) == 0 {
		return
	}
	existing := make(map[string]any)
	if raw, ok := block["envs"].(map[string]any); ok {
		for k, v := range raw {
			existing[k] = v
		}
	}
	keys := make([]string, 0, len(overlay))
	for k := range overlay {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, has := existing[k]; !has {
			existing[k] = overlay[k]
		}
	}
	block["envs"] = existing
}

// mergeCABlock appends overlay CA entries, deduplicating by digest value.
// Block entries appear first (higher precedence).
func mergeCABlock(block map[string]any, overlay []string) {
	if len(overlay) == 0 {
		return
	}
	seen := make(map[string]bool)
	var merged []any

	if raw, ok := block["ca"].([]any); ok {
		for _, e := range raw {
			s, ok := e.(string)
			if !ok {
				merged = append(merged, e)
				continue
			}
			key := hydraCADedup(s)
			if !seen[key] {
				seen[key] = true
				merged = append(merged, s)
			}
		}
	}

	for _, s := range overlay {
		key := hydraCADedup(s)
		if !seen[key] {
			seen[key] = true
			merged = append(merged, s)
		}
	}

	if len(merged) > 0 {
		block["ca"] = merged
	}
}

// hydraCADedup returns a dedup key for a CA entry. Hex-hash entries (7-64
// chars) are used as-is; file paths are hashed for comparison.
func hydraCADedup(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 7 && len(s) <= 64 && isHexStr(s) {
		return s
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func isHexStr(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// mergeRecordsByDstBlock merges overlay entries into block[field] by
// destination. Block entries win when the destination matches.
func mergeRecordsByDstBlock(block map[string]any, field string, overlay []string) {
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
			dst := hydraExtractDst(field, s)
			specDsts[dst] = true
			merged = append(merged, s)
		}
	}

	for _, s := range overlay {
		dst := hydraExtractDst(field, s)
		if !specDsts[dst] {
			specDsts[dst] = true
			merged = append(merged, s)
		}
	}

	if len(merged) > 0 {
		block[field] = merged
	}
}

// hydraExtractDst extracts the normalized destination from a Hydra entry
// using first-colon split semantics matching contracts.splitHashDst.
// For in/out: dst is everything after the first colon (shortHash:dst).
// For home: body after trimming :ro suffix is split at the first colon,
// then normalized with path.Clean so equivalent paths like ".config//app"
// and ".config/app" dedup correctly.
func hydraExtractDst(field, entry string) string {
	switch field {
	case "home":
		body := strings.TrimSuffix(entry, ":ro")
		idx := strings.Index(body, ":")
		if idx >= 0 {
			return path.Clean(body[idx+1:])
		}
		return path.Clean(body)
	default: // in, out
		idx := strings.Index(entry, ":")
		if idx >= 0 {
			return entry[idx+1:]
		}
		return entry
	}
}

// validateOverlayCollisions checks for duplicate destinations within a single
// overlay's in, out, and home fields. Returns a deterministic error listing
// all collisions found.
func validateOverlayCollisions(cfg *HydraJobConfig, prefix string) error {
	if cfg == nil {
		return nil
	}
	var errs []string
	for _, f := range []struct {
		name    string
		entries []string
	}{
		{"in", cfg.In},
		{"out", cfg.Out},
		{"home", cfg.Home},
	} {
		if dups := findDuplicateDsts(f.name, f.entries); len(dups) > 0 {
			for _, dst := range dups {
				errs = append(errs, fmt.Sprintf("%s.%s: duplicate destination %q", prefix, f.name, dst))
			}
		}
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("hydra overlay collision: %s", strings.Join(errs, "; "))
	}
	return nil
}

// findDuplicateDsts returns sorted destination strings that appear more than
// once in the given entries.
func findDuplicateDsts(field string, entries []string) []string {
	seen := make(map[string]int)
	for _, e := range entries {
		dst := hydraExtractDst(field, e)
		seen[dst]++
	}
	var dups []string
	for dst, count := range seen {
		if count > 1 {
			dups = append(dups, dst)
		}
	}
	sort.Strings(dups)
	return dups
}

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func copyStringSlice(s []string) []string {
	if s == nil {
		return nil
	}
	cp := make([]string, len(s))
	copy(cp, s)
	return cp
}

// applyBundleMapMutator merges server-side bundle mappings (from migration and
// config APIs) into spec["bundle_map"]. Existing spec entries take precedence.
func applyBundleMapMutator(m map[string]any, in claimSpecMutatorInput) error {
	if len(in.bundleMap) == 0 {
		return nil
	}

	existing := make(map[string]string)
	switch raw := m["bundle_map"].(type) {
	case map[string]string:
		for k, v := range raw {
			existing[k] = v
		}
	case map[string]any:
		for k, v := range raw {
			if s, ok := v.(string); ok {
				existing[k] = s
			}
		}
	}

	for hash, bundleID := range in.bundleMap {
		if _, has := existing[hash]; !has {
			existing[hash] = bundleID
		}
	}

	if len(existing) > 0 {
		m["bundle_map"] = existing
	}
	return nil
}

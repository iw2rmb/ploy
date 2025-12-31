// claimer_spec.go isolates spec JSON payload parsing from claim orchestration.
//
// This file contains parseSpec which decodes run specifications from the
// control plane claim response. It uses the canonical contracts.ParseModsSpecJSON
// parser for structured validation and then converts to the internal RunOptions
// format. Separating spec parsing from claim logic enables focused testing of
// the decoding contract without coupling to HTTP claim mechanics.
package nodeagent

import (
	"encoding/json"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// stringValue safely dereferences a string pointer, returning empty string if nil.
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseSpec splits a spec JSON payload into options and environment maps.
// It uses the canonical contracts.ParseModsSpecJSON parser for structured
// validation, then converts to the internal RunOptions format.
//
// The spec is expected to contain fields like "image", "command", "env",
// "build_gate", "mods", and other configuration values. This function extracts
// and flattens nested structures according to the following rules:
//
//   - Top-level fields like "image", "command", "env" are extracted directly.
//   - The "build_gate" object is flattened into "build_gate_enabled" and
//     "build_gate_profile" options for manifest builder consumption.
//   - Server-injected metadata like "job_id", "gitlab_pat", "gitlab_domain",
//     "mr_on_success", and "mr_on_fail" are passed through as-is.
//   - The "build_gate_healing" block is preserved in options to support heal → re-gate loops.
//   - For multi-step runs, the "mods[]" array is preserved for step-by-step execution.
//
// ## Canonical Spec Shapes
//
// Single-step runs use top-level fields:
//
//	{"image": "...", "command": "...", "env": {...}, "build_gate": {...}}
//
// Multi-step runs use the mods[] array:
//
//	{"mods": [{...}, {...}], "build_gate": {...}, "build_gate_healing": {...}}
//
// The legacy "mod" object fallback (where mod.image, mod.command, etc. were copied
// to top-level when missing) is no longer supported. Specs must use one of the
// canonical shapes above.
//
// ## Return Values
//
// Returns:
//   - opts: map[string]any containing flattened options. This is an internal
//     intermediate representation used to bridge JSON parsing and typed options.
//     Callers should use typedOpts for all option access.
//   - env: map[string]string containing merged environment variables.
//   - typedOpts: RunOptions with typed accessors for all understood option keys.
//     This is the canonical source of truth; prefer typed fields over raw map access.
//
// If the spec is empty or invalid JSON, returns empty maps and zero RunOptions.
func parseSpec(spec json.RawMessage) (map[string]any, map[string]string, RunOptions) {
	opts := map[string]any{}
	env := map[string]string{}
	var typedOpts RunOptions
	if len(spec) == 0 {
		return opts, env, typedOpts
	}

	// Parse using the canonical parser for structural validation.
	// We still need the raw map for server-injected fields (job_id, mod_index)
	// that aren't part of the canonical spec schema.
	modsSpec, err := contracts.ParseModsSpecJSON(spec)
	if err != nil {
		// Fallback to raw map parsing for backwards compatibility.
		// This allows specs with server-injected fields to still be processed.
		var root any
		if jsonErr := json.Unmarshal(spec, &root); jsonErr != nil {
			return opts, env, typedOpts
		}
		m, ok := root.(map[string]any)
		if !ok {
			return map[string]any{"spec": root}, env, typedOpts
		}
		// Use legacy parsing path for malformed specs.
		return parseSpecFromRawMap(m)
	}

	// Also parse raw map to extract server-injected fields not in canonical schema.
	var rawMap map[string]any
	if err := json.Unmarshal(spec, &rawMap); err != nil {
		rawMap = map[string]any{}
	}

	// Convert canonical ModsSpec to internal options format.
	opts, env = modsSpecToOptions(modsSpec, rawMap)

	// Parse typed options from the flattened opts map.
	typedOpts = parseRunOptions(opts)

	return opts, env, typedOpts
}

// modsSpecToOptions converts a canonical ModsSpec to the internal opts/env format.
// It also extracts server-injected fields (job_id, mod_index) from rawMap.
//
// This function preserves the original types from rawMap where possible to maintain
// backwards compatibility with existing test expectations (e.g., float64 for retries,
// []any for command arrays).
func modsSpecToOptions(spec *contracts.ModsSpec, rawMap map[string]any) (map[string]any, map[string]string) {
	opts := make(map[string]any)
	env := make(map[string]string)

	// Extract env from spec (single-step runs).
	for k, v := range spec.Env {
		env[k] = v
	}

	// Convert image to opts format.
	if !spec.Image.IsEmpty() {
		if spec.Image.Universal != "" {
			opts["image"] = spec.Image.Universal
		} else if len(spec.Image.ByStack) > 0 {
			imgMap := make(map[string]any, len(spec.Image.ByStack))
			for k, v := range spec.Image.ByStack {
				imgMap[string(k)] = v
			}
			opts["image"] = imgMap
		}
	}

	// Convert command to opts format.
	// Preserve []any for exec arrays (backwards compat with JSON unmarshaling).
	if !spec.Command.IsEmpty() {
		if len(spec.Command.Exec) > 0 {
			// Convert to []any for backwards compatibility with JSON unmarshaling.
			cmdSlice := make([]any, len(spec.Command.Exec))
			for i, s := range spec.Command.Exec {
				cmdSlice[i] = s
			}
			opts["command"] = cmdSlice
		} else if spec.Command.Shell != "" {
			opts["command"] = spec.Command.Shell
		}
	}

	// Retain container.
	if spec.RetainContainer {
		opts["retain_container"] = true
	}

	// Build gate - flatten enabled/profile and set enabled even when false.
	if spec.BuildGate != nil {
		// Always set build_gate_enabled when BuildGate is present (including false).
		opts["build_gate_enabled"] = spec.BuildGate.Enabled
		if spec.BuildGate.Profile != "" {
			opts["build_gate_profile"] = spec.BuildGate.Profile
		}
	}

	// Build gate healing - preserve original types from rawMap for backwards compat.
	// Tests expect float64 for retries and []any for command arrays.
	if healing, ok := rawMap["build_gate_healing"].(map[string]any); ok && len(healing) > 0 {
		opts["build_gate_healing"] = healing
	}

	// Multi-step mods - preserve original mods[] array from rawMap.
	// Tests expect []any with map[string]any entries preserving original types.
	if modsSlice, ok := rawMap["mods"].([]any); ok && len(modsSlice) > 0 {
		opts["mods"] = modsSlice
	}

	// GitLab integration.
	if spec.GitLabPAT != "" {
		opts["gitlab_pat"] = spec.GitLabPAT
	}
	if spec.GitLabDomain != "" {
		opts["gitlab_domain"] = spec.GitLabDomain
	}
	if spec.MROnSuccess {
		opts["mr_on_success"] = true
	}
	if spec.MROnFail {
		opts["mr_on_fail"] = true
	}

	// Artifact configuration.
	if spec.ArtifactName != "" {
		opts["artifact_name"] = spec.ArtifactName
	}
	if len(spec.ArtifactPaths) > 0 {
		opts["artifact_paths"] = spec.ArtifactPaths
	}

	// Extract server-injected fields from raw map (not part of canonical spec).
	if jid, ok := rawMap["job_id"].(string); ok && jid != "" {
		opts["job_id"] = jid
	}
	if mi, ok := rawMap["mod_index"]; ok {
		switch v := mi.(type) {
		case float64:
			opts["mod_index"] = int(v)
		case int:
			opts["mod_index"] = v
		}
	}

	return opts, env
}

// parseSpecFromRawMap is the legacy parsing path used when canonical parsing fails.
// This preserves backwards compatibility with specs that have server-injected fields
// or other non-canonical structures.
func parseSpecFromRawMap(m map[string]any) (map[string]any, map[string]string, RunOptions) {
	opts := map[string]any{}
	env := map[string]string{}

	// Extract known fields at top level.
	// Image may be a string (universal) or a map (stack-aware).
	if v, ok := m["image"]; ok && v != nil {
		switch img := v.(type) {
		case string:
			if img != "" {
				opts["image"] = img
			}
		case map[string]any:
			if len(img) > 0 {
				opts["image"] = img
			}
		case map[string]string:
			if len(img) > 0 {
				opts["image"] = img
			}
		}
	}
	if v, ok := m["command"].(string); ok && v != "" {
		opts["command"] = v
	}
	if arr, ok := m["command"].([]any); ok && len(arr) > 0 {
		out := make([]string, 0, len(arr))
		for _, e := range arr {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			opts["command"] = out
		}
	}
	if e, ok := m["env"].(map[string]any); ok {
		for k, v := range e {
			if s, ok := v.(string); ok {
				env[k] = s
			}
		}
	}
	// Optional retain flag for container lifecycle.
	if b, ok := m["retain_container"].(bool); ok {
		opts["retain_container"] = b
	}
	// Pass through job_id if present (server injects it on claim).
	if jid, ok := m["job_id"].(string); ok && jid != "" {
		opts["job_id"] = jid
	}
	// Pass through GitLab config (PAT and domain) if present.
	if pat, ok := m["gitlab_pat"].(string); ok && pat != "" {
		opts["gitlab_pat"] = pat
	}
	if domain, ok := m["gitlab_domain"].(string); ok && domain != "" {
		opts["gitlab_domain"] = domain
	}
	// Pass through MR creation flags if present.
	if mrSuccess, ok := m["mr_on_success"].(bool); ok {
		opts["mr_on_success"] = mrSuccess
	}
	if mrFail, ok := m["mr_on_fail"].(bool); ok {
		opts["mr_on_fail"] = mrFail
	}

	// Pass through build_gate_healing block if present.
	if healing, ok := m["build_gate_healing"]; ok {
		switch h := healing.(type) {
		case map[string]any:
			if len(h) > 0 {
				opts["build_gate_healing"] = h
			}
		case []any:
			if len(h) > 0 {
				opts["build_gate_healing"] = h
			}
		default:
			if healing != nil {
				opts["build_gate_healing"] = healing
			}
		}
	}

	// Flatten build_gate.enabled/profile for manifest builder to honor.
	if bg, ok := m["build_gate"].(map[string]any); ok {
		if b, ok := bg["enabled"].(bool); ok {
			opts["build_gate_enabled"] = b
		}
		if p, ok := bg["profile"].(string); ok && p != "" {
			opts["build_gate_profile"] = p
		}
	}

	// Pass through mods[] array for multi-step run execution.
	if modsSlice, ok := m["mods"].([]any); ok && len(modsSlice) > 0 {
		opts["mods"] = modsSlice
	}

	// Pass through mod_index when present.
	if mi, ok := m["mod_index"]; ok {
		switch v := mi.(type) {
		case float64:
			opts["mod_index"] = int(v)
		case int:
			opts["mod_index"] = v
		}
	}

	// Parse typed options from the flattened opts map.
	typedOpts := parseRunOptions(opts)

	return opts, env, typedOpts
}

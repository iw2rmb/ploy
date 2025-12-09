// claimer_spec.go isolates spec JSON payload parsing from claim orchestration.
//
// This file contains parseSpec which decodes run specifications from the
// control plane claim response. It flattens nested structures (build_gate,
// env, options) and extracts configuration for healing, MR creation, and
// mod execution. Separating spec parsing from claim logic enables focused
// testing of the decoding contract without coupling to HTTP claim mechanics.
package nodeagent

import (
	"encoding/json"
)

// stringValue safely dereferences a string pointer, returning empty string if nil.
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseSpec splits a spec JSON payload into options and environment maps.
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
// Returns:
//   - opts: map[string]any containing flattened options (preserved for raw JSON access).
//   - env: map[string]string containing merged environment variables.
//   - typedOpts: RunOptions with typed accessors for all understood option keys.
//
// If the spec is empty or invalid JSON, returns empty maps and zero RunOptions.
func parseSpec(spec json.RawMessage) (map[string]any, map[string]string, RunOptions) {
	opts := map[string]any{}
	env := map[string]string{}
	var typedOpts RunOptions
	if len(spec) == 0 {
		return opts, env, typedOpts
	}
	var root any
	if err := json.Unmarshal(spec, &root); err != nil {
		return opts, env, typedOpts
	}
	m, ok := root.(map[string]any)
	if !ok {
		return map[string]any{"spec": root}, env, typedOpts
	}
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
	// Pass through GitLab config (PAT and domain) if present (server injects defaults on claim).
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

	// Pass through build_gate_healing block if present so the agent can
	// execute the heal → re‑gate loop when the initial gate fails.
	// Accept either a map (decoded object) or arbitrary JSON; store as-is.
	if healing, ok := m["build_gate_healing"]; ok {
		// Only include non-empty objects/arrays to avoid confusing downstream logic.
		switch h := healing.(type) {
		case map[string]any:
			if len(h) > 0 {
				opts["build_gate_healing"] = h
			}
		case []any:
			// Unlikely, but preserve if provided (defensive).
			if len(h) > 0 {
				opts["build_gate_healing"] = h
			}
		default:
			// Preserve scalar values only when not empty.
			if healing != nil {
				opts["build_gate_healing"] = healing
			}
		}
	}

	// NOTE: Legacy "mod" object fallback has been removed. Specs must use either:
	// - Top-level fields (image, command, env, etc.) for single-step runs, OR
	// - The mods[] array for multi-step runs.
	// The "mod" object (e.g., {"mod": {"image": "...", "env": {...}}}) is no longer
	// processed; such specs must be migrated to the canonical shapes above.

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
	// For multi-step runs (mods[] in spec), preserve the array for step-by-step execution.
	// Each entry in mods[] defines a gate+mod step with its own image, command, and env.
	if modsSlice, ok := m["mods"].([]any); ok && len(modsSlice) > 0 {
		opts["mods"] = modsSlice
	}

	// Pass through mod_index when present. This is a server-injected per-job
	// index that maps mod jobs to mods[mod_index] in multi-step specs.
	if mi, ok := m["mod_index"]; ok {
		switch v := mi.(type) {
		case float64:
			opts["mod_index"] = int(v)
		case int:
			opts["mod_index"] = v
		}
	}

	// Parse typed options from the flattened opts map.
	// This provides type-safe accessors for all understood option keys while
	// preserving the raw map for backward compatibility and wire-level JSON access.
	typedOpts = parseRunOptions(opts)

	return opts, env, typedOpts
}

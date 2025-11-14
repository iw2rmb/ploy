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
// The spec is expected to contain fields like "image", "command", "env", "mod",
// "build_gate", and other configuration values. This function extracts and flattens
// nested structures according to the following rules:
//
//   - Top-level fields like "image", "command", "env" are extracted directly.
//   - The "mod" object provides fallback values: if a top-level field is missing,
//     the corresponding "mod" field is used instead. Top-level always takes precedence.
//   - Environment variables from both top-level "env" and "mod.env" are merged,
//     with top-level "env" winning on conflict.
//   - The "build_gate" object is flattened into "build_gate_enabled" and
//     "build_gate_profile" options for manifest builder consumption.
//   - Server-injected metadata like "stage_id", "gitlab_pat", "gitlab_domain",
//     "mr_on_success", and "mr_on_fail" are passed through as-is.
//   - The "build_gate_healing" block is preserved in options to support heal → re-gate loops.
//
// Returns two maps:
//   - opts: map[string]any containing flattened options for run execution.
//   - env: map[string]string containing merged environment variables.
//
// If the spec is empty or invalid JSON, returns empty maps.
func parseSpec(spec json.RawMessage) (map[string]any, map[string]string) {
	opts := map[string]any{}
	env := map[string]string{}
	if len(spec) == 0 {
		return opts, env
	}
	var root any
	if err := json.Unmarshal(spec, &root); err != nil {
		return opts, env
	}
	m, ok := root.(map[string]any)
	if !ok {
		return map[string]any{"spec": root}, env
	}
	// Extract known fields at top level.
	if v, ok := m["image"].(string); ok && v != "" {
		opts["image"] = v
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
	// Pass through stage_id if present (server injects it on claim).
	if sid, ok := m["stage_id"].(string); ok && sid != "" {
		opts["stage_id"] = sid
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

	// Flatten nested mod.* into options/env (top-level values take precedence).
	if mod, ok := m["mod"].(map[string]any); ok {
		// image
		if _, present := opts["image"]; !present {
			if v, ok := mod["image"].(string); ok && v != "" {
				opts["image"] = v
			}
		}
		// command (string or array)
		if _, present := opts["command"]; !present {
			switch v := mod["command"].(type) {
			case []any:
				out := make([]string, 0, len(v))
				for _, e := range v {
					if s, ok := e.(string); ok {
						out = append(out, s)
					}
				}
				if len(out) > 0 {
					opts["command"] = out
				}
			case string:
				if s := v; s != "" {
					opts["command"] = s
				}
			}
		}
		// retain_container
		if _, present := opts["retain_container"]; !present {
			if b, ok := mod["retain_container"].(bool); ok {
				opts["retain_container"] = b
			}
		}
		// env merge (top-level env wins on conflict)
		if em, ok := mod["env"].(map[string]any); ok {
			for k, v := range em {
				if s, ok := v.(string); ok {
					if _, exists := env[k]; !exists {
						env[k] = s
					}
				}
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
	return opts, env
}

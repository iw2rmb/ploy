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
	"strings"

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
// The spec is expected to contain fields like "steps", "env",
// "build_gate", and other configuration values. This function extracts
// and flattens nested structures according to the following rules:
//
//   - Single-step execution fields are taken from steps[0] (image, command, retain_container).
//   - The "build_gate" object is flattened into "build_gate_enabled" and
//     "build_gate_profile" options for manifest builder consumption.
//   - Server-injected metadata like "job_id", "gitlab_pat", "gitlab_domain",
//     "mr_on_success", and "mr_on_fail" are passed through as-is.
//   - The "build_gate.healing" block is preserved in options to support heal → re-gate loops.
//   - For multi-step runs, the "steps[]" array is preserved for step-by-step execution.
//
// ## Canonical Spec Shape
//
// All runs use steps[] (even single-step runs).
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
	modsSpec, err := contracts.ParseModsSpecJSON(spec)
	if err != nil {
		// If spec is invalid, return empty opts/env and let downstream execution fail fast
		// when required fields are missing.
		return opts, env, typedOpts
	}

	// Convert canonical ModsSpec to internal options format.
	opts, env = modsSpecToOptions(modsSpec)

	// Parse typed options from the flattened opts map.
	typedOpts = parseRunOptions(opts)

	return opts, env, typedOpts
}

func modImageToAny(img contracts.ModImage) any {
	if img.Universal != "" {
		return img.Universal
	}
	if len(img.ByStack) > 0 {
		out := make(map[string]any, len(img.ByStack))
		for k, v := range img.ByStack {
			out[string(k)] = v
		}
		return out
	}
	return nil
}

func commandSpecToAnyForNested(cmd contracts.CommandSpec) any {
	if len(cmd.Exec) > 0 {
		out := make([]any, 0, len(cmd.Exec))
		for _, s := range cmd.Exec {
			out = append(out, s)
		}
		return out
	}
	if cmd.Shell != "" {
		return cmd.Shell
	}
	return nil
}

func stringMapToAnyMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// modsSpecToOptions converts a canonical ModsSpec to the internal opts/env format.
func modsSpecToOptions(spec *contracts.ModsSpec) (map[string]any, map[string]string) {
	opts := make(map[string]any)
	env := make(map[string]string)

	// Extract base env.
	for k, v := range spec.Env {
		env[k] = v
	}

	// Build gate - flatten enabled/profile and set enabled even when false.
	if spec.BuildGate != nil {
		// Always set build_gate_enabled when BuildGate is present (including false).
		opts["build_gate_enabled"] = spec.BuildGate.Enabled
		if spec.BuildGate.Profile != "" {
			opts["build_gate_profile"] = spec.BuildGate.Profile
		}

		bg := make(map[string]any)
		bg["enabled"] = spec.BuildGate.Enabled
		if spec.BuildGate.Profile != "" {
			bg["profile"] = spec.BuildGate.Profile
		}
		if spec.BuildGate.Healing != nil {
			healing := make(map[string]any)
			healing["retries"] = spec.BuildGate.Healing.Retries
			if spec.BuildGate.Healing.Mod != nil {
				mod := make(map[string]any)
				if !spec.BuildGate.Healing.Mod.Image.IsEmpty() {
					mod["image"] = modImageToAny(spec.BuildGate.Healing.Mod.Image)
				}
				if !spec.BuildGate.Healing.Mod.Command.IsEmpty() {
					mod["command"] = commandSpecToAnyForNested(spec.BuildGate.Healing.Mod.Command)
				}
				if len(spec.BuildGate.Healing.Mod.Env) > 0 {
					mod["env"] = stringMapToAnyMap(spec.BuildGate.Healing.Mod.Env)
				}
				if spec.BuildGate.Healing.Mod.RetainContainer {
					mod["retain_container"] = true
				}
				healing["mod"] = mod
			}
			bg["healing"] = healing
		}
		opts["build_gate"] = bg
	}

	// Steps.
	if len(spec.Steps) == 1 {
		step := spec.Steps[0]
		if !step.Image.IsEmpty() {
			opts["image"] = modImageToAny(step.Image)
		}
		if !step.Command.IsEmpty() {
			opts["command"] = commandSpecToAnyForNested(step.Command)
		}
		if step.RetainContainer {
			opts["retain_container"] = true
		}
		for k, v := range step.Env {
			env[k] = v
		}
	} else if len(spec.Steps) > 1 {
		steps := make([]any, 0, len(spec.Steps))
		for _, step := range spec.Steps {
			m := make(map[string]any)
			if strings.TrimSpace(step.Name) != "" {
				m["name"] = strings.TrimSpace(step.Name)
			}
			if !step.Image.IsEmpty() {
				m["image"] = modImageToAny(step.Image)
			}
			if !step.Command.IsEmpty() {
				m["command"] = commandSpecToAnyForNested(step.Command)
			}
			if len(step.Env) > 0 {
				m["env"] = stringMapToAnyMap(step.Env)
			}
			if step.RetainContainer {
				m["retain_container"] = true
			}
			steps = append(steps, m)
		}
		opts["steps"] = steps
	}

	// GitLab integration.
	if spec.GitLabPAT != "" {
		opts["gitlab_pat"] = spec.GitLabPAT
	}
	if spec.GitLabDomain != "" {
		opts["gitlab_domain"] = spec.GitLabDomain
	}
	if spec.MROnSuccess != nil {
		opts["mr_on_success"] = *spec.MROnSuccess
	}
	if spec.MROnFail != nil {
		opts["mr_on_fail"] = *spec.MROnFail
	}

	// Artifact configuration.
	if spec.ArtifactName != "" {
		opts["artifact_name"] = spec.ArtifactName
	}
	if len(spec.ArtifactPaths) > 0 {
		opts["artifact_paths"] = spec.ArtifactPaths
	}

	// Server-injected fields.
	if strings.TrimSpace(spec.JobID) != "" {
		opts["job_id"] = strings.TrimSpace(spec.JobID)
	}
	if spec.ModIndex != nil {
		opts["mod_index"] = *spec.ModIndex
	}

	return opts, env
}

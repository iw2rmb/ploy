// mods_spec_parse.go provides JSON parsing for Mods specifications.
//
// The parsing strategy handles polymorphic fields (image, command) that can appear
// as either strings or structured objects in the spec JSON. Standard JSON unmarshaling
// cannot handle this directly, so we parse into map[string]any first and then
// convert to typed structures.
//
// Usage:
//
//	spec, err := contracts.ParseModsSpecJSON(jsonBytes)
//	if err != nil {
//	    return err // structured validation error with field paths
//	}
//
// YAML files are accepted at the CLI boundary by loading into map[string]any,
// marshaling to JSON, and validating via ParseModsSpecJSON.
package contracts

import (
	"encoding/json"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// ParseModsSpecJSON parses a Mods specification from JSON bytes.
// Returns a validated ModsSpec or an error for invalid/malformed input.
//
// Errors are structured with field paths for actionable debugging:
//   - "steps[2].image: required" — missing required field in step
//   - "build_gate.healing.retries: must be non-negative" — invalid value
func ParseModsSpecJSON(data []byte) (*ModsSpec, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("steps: required")
	}

	// Unmarshal into intermediate map to handle polymorphic fields.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse migs spec json: %w", err)
	}

	return parseModsSpecFromMap(raw)
}

// parseModsSpecFromMap converts a raw map to a typed ModsSpec.
// This shared implementation handles polymorphic field parsing (image, command)
// that requires special handling beyond standard JSON/YAML unmarshaling.
func parseModsSpecFromMap(raw map[string]any) (*ModsSpec, error) {
	spec := &ModsSpec{}

	// Parse server-injected fields.
	if v, ok := raw["job_id"]; ok && v != nil {
		s, err := expectString(v, "job_id")
		if err != nil {
			return nil, err
		}
		s = strings.TrimSpace(s)
		if s != "" {
			var id types.JobID
			if err := id.UnmarshalText([]byte(s)); err != nil {
				return nil, fmt.Errorf("job_id: %w", err)
			}
			spec.JobID = id
		}
	}
	if _, ok := raw["mod_index"]; ok {
		return nil, fmt.Errorf("mod_index: forbidden (derived internally from next_id; must not be provided)")
	}

	// Parse optional metadata fields.
	if v, ok := raw["apiVersion"]; ok && v != nil {
		s, err := expectString(v, "apiVersion")
		if err != nil {
			return nil, err
		}
		spec.APIVersion = strings.TrimSpace(s)
	}
	if v, ok := raw["kind"]; ok && v != nil {
		s, err := expectString(v, "kind")
		if err != nil {
			return nil, err
		}
		spec.Kind = strings.TrimSpace(s)
	}

	// Parse top-level env.
	if v, ok := raw["env"]; ok && v != nil {
		env, err := parseEnvMap(v, "env")
		if err != nil {
			return nil, err
		}
		spec.Env = env
	}

	// Parse steps[] array (required).
	v, ok := raw["steps"]
	if !ok || v == nil {
		return nil, fmt.Errorf("steps: required")
	}
	stepsRaw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("steps: expected array, got %T", v)
	}
	if len(stepsRaw) == 0 {
		return nil, fmt.Errorf("steps: required")
	}
	spec.Steps = make([]ModStep, 0, len(stepsRaw))
	for i, stepRaw := range stepsRaw {
		stepMap, ok := stepRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("steps[%d]: expected object, got %T", i, stepRaw)
		}
		step, err := parseModStep(stepMap, i)
		if err != nil {
			return nil, err
		}
		spec.Steps = append(spec.Steps, step)
	}

	// Parse build_gate.
	if v, ok := raw["build_gate"]; ok && v != nil {
		bgRaw, err := expectMap(v, "build_gate")
		if err != nil {
			return nil, err
		}
		bg := &BuildGateConfig{}
		if _, ok := bgRaw["profile"]; ok {
			return nil, fmt.Errorf("build_gate.profile: forbidden")
		}
		if vv, ok := bgRaw["enabled"]; ok && vv != nil {
			b, err := expectBool(vv, "build_gate.enabled")
			if err != nil {
				return nil, err
			}
			bg.Enabled = b
		}
		if vv, ok := bgRaw["healing"]; ok && vv != nil {
			healRaw, err := expectMap(vv, "build_gate.healing")
			if err != nil {
				return nil, err
			}
			heal, err := parseHealingSpec(healRaw, "build_gate.healing")
			if err != nil {
				return nil, err
			}
			bg.Healing = heal
		}
		if vv, ok := bgRaw["router"]; ok && vv != nil {
			routerRaw, err := expectMap(vv, "build_gate.router")
			if err != nil {
				return nil, err
			}
			router, err := parseRouterSpec(routerRaw, "build_gate.router")
			if err != nil {
				return nil, err
			}
			bg.Router = router
		}
		if vv, ok := bgRaw["images"]; ok && vv != nil {
			imagesRaw, ok := vv.([]any)
			if !ok {
				return nil, fmt.Errorf("build_gate.images: expected array, got %T", vv)
			}
			images, err := parseBuildGateImageRules(imagesRaw, "build_gate.images")
			if err != nil {
				return nil, err
			}
			bg.Images = images
		}
		if vv, ok := bgRaw["pre"]; ok && vv != nil {
			preRaw, err := expectMap(vv, "build_gate.pre")
			if err != nil {
				return nil, err
			}
			pre, err := parseBuildGatePhaseConfig(preRaw, "build_gate.pre")
			if err != nil {
				return nil, err
			}
			bg.Pre = pre
		}
		if vv, ok := bgRaw["post"]; ok && vv != nil {
			postRaw, err := expectMap(vv, "build_gate.post")
			if err != nil {
				return nil, err
			}
			post, err := parseBuildGatePhaseConfig(postRaw, "build_gate.post")
			if err != nil {
				return nil, err
			}
			bg.Post = post
		}
		spec.BuildGate = bg
	}

	// Parse GitLab integration.
	if v, ok := raw["gitlab_pat"]; ok && v != nil {
		s, err := expectString(v, "gitlab_pat")
		if err != nil {
			return nil, err
		}
		spec.GitLabPAT = s
	}
	if v, ok := raw["gitlab_domain"]; ok && v != nil {
		s, err := expectString(v, "gitlab_domain")
		if err != nil {
			return nil, err
		}
		spec.GitLabDomain = strings.TrimSpace(s)
	}
	if v, ok := raw["mr_on_success"]; ok && v != nil {
		b, err := expectBool(v, "mr_on_success")
		if err != nil {
			return nil, err
		}
		spec.MROnSuccess = &b
	}
	if v, ok := raw["mr_on_fail"]; ok && v != nil {
		b, err := expectBool(v, "mr_on_fail")
		if err != nil {
			return nil, err
		}
		spec.MROnFail = &b
	}

	// Parse artifact configuration.
	if v, ok := raw["artifact_name"]; ok && v != nil {
		s, err := expectString(v, "artifact_name")
		if err != nil {
			return nil, err
		}
		spec.ArtifactName = strings.TrimSpace(s)
	}
	if pathsRaw, ok := raw["artifact_paths"]; ok && pathsRaw != nil {
		paths, err := parseStringSlice(pathsRaw, "artifact_paths")
		if err != nil {
			return nil, err
		}
		spec.ArtifactPaths = paths
	}

	// Normalize defaults.
	if strings.TrimSpace(spec.GitLabPAT) != "" && strings.TrimSpace(spec.GitLabDomain) == "" {
		spec.GitLabDomain = "gitlab.com"
	}

	// Validate the parsed spec.
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	return spec, nil
}

// parseModStep parses a single mig step entry from the steps[] array.
func parseModStep(raw map[string]any, index int) (ModStep, error) {
	step := ModStep{}
	prefix := fmt.Sprintf("steps[%d]", index)

	// Parse optional name.
	if v, ok := raw["name"]; ok && v != nil {
		s, err := expectString(v, prefix+".name")
		if err != nil {
			return step, err
		}
		step.Name = strings.TrimSpace(s)
	}

	// Parse shared mig-like fields (image, command, env, retain_container).
	f, err := parseModLikeFields(raw, prefix)
	if err != nil {
		return step, err
	}
	step.Image = f.Image
	step.Command = f.Command
	step.Env = f.Env
	step.RetainContainer = f.RetainContainer

	// Parse stack gate configuration.
	if v, ok := raw["stack"]; ok && v != nil {
		stackRaw, err := expectMap(v, prefix+".stack")
		if err != nil {
			return step, err
		}
		stack, err := parseStackGateSpec(stackRaw, prefix+".stack")
		if err != nil {
			return step, err
		}
		step.Stack = stack
	}

	return step, nil
}

func parseHealingSpec(raw map[string]any, prefix string) (*HealingSpec, error) {
	heal := &HealingSpec{
		Retries: 1, // Default to 1 retry.
	}

	// Parse retries (handle both int and float64 from JSON).
	if v, ok := raw["retries"]; ok && v != nil {
		if r, ok := v.(int); ok {
			heal.Retries = r
		} else if rf, ok := v.(float64); ok {
			heal.Retries = int(rf)
		} else {
			return nil, fmt.Errorf("%s.retries: expected number, got %T", prefix, v)
		}
	}

	// Parse mig-like fields directly on the healing map (flattened form).
	f, err := parseModLikeFields(raw, prefix)
	if err != nil {
		return nil, err
	}
	heal.Image = f.Image
	heal.Command = f.Command
	heal.Env = f.Env
	heal.RetainContainer = f.RetainContainer

	return heal, nil
}

func parseRouterSpec(raw map[string]any, prefix string) (*RouterSpec, error) {
	f, err := parseModLikeFields(raw, prefix)
	if err != nil {
		return nil, err
	}
	return &RouterSpec{
		Image:           f.Image,
		Command:         f.Command,
		Env:             f.Env,
		RetainContainer: f.RetainContainer,
	}, nil
}

func parseBuildGatePhaseConfig(raw map[string]any, prefix string) (*BuildGatePhaseConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	phase := &BuildGatePhaseConfig{}

	if v, ok := raw["stack"]; ok && v != nil {
		stackRaw, err := expectMap(v, prefix+".stack")
		if err != nil {
			return nil, err
		}
		stack, err := parseBuildGateStackConfig(stackRaw, prefix+".stack")
		if err != nil {
			return nil, err
		}
		phase.Stack = stack
	}

	if phase.Stack == nil {
		return nil, nil
	}

	return phase, nil
}

func parseBuildGateStackConfig(raw map[string]any, prefix string) (*BuildGateStackConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	stack := &BuildGateStackConfig{}

	if v, ok := raw["enabled"]; ok && v != nil {
		b, err := expectBool(v, prefix+".enabled")
		if err != nil {
			return nil, err
		}
		stack.Enabled = b
	}

	if v, ok := raw["default"]; ok && v != nil {
		b, err := expectBool(v, prefix+".default")
		if err != nil {
			return nil, err
		}
		stack.Default = b
	}

	if v, ok := raw["language"]; ok && v != nil {
		s, err := expectString(v, prefix+".language")
		if err != nil {
			return nil, err
		}
		stack.Language = strings.TrimSpace(s)
	}

	if v, ok := raw["tool"]; ok && v != nil {
		s, err := expectString(v, prefix+".tool")
		if err != nil {
			return nil, err
		}
		stack.Tool = strings.TrimSpace(s)
	}

	if v, ok := raw["release"]; ok && v != nil {
		release, err := parseReleaseValue(v, prefix+".release")
		if err != nil {
			return nil, err
		}
		stack.Release = release
	}

	return stack, nil
}

// parseCommandSpec parses a command from polymorphic input (string or array).
func parseCommandSpec(v any) (CommandSpec, error) {
	switch cmd := v.(type) {
	case string:
		return CommandSpec{Shell: strings.TrimSpace(cmd)}, nil
	case []any:
		exec := make([]string, 0, len(cmd))
		for _, elem := range cmd {
			s, ok := elem.(string)
			if !ok {
				return CommandSpec{}, fmt.Errorf("expected string array element, got %T", elem)
			}
			exec = append(exec, s)
		}
		return CommandSpec{Exec: exec}, nil
	case []string:
		return CommandSpec{Exec: cmd}, nil
	default:
		return CommandSpec{}, fmt.Errorf("expected string or array, got %T", v)
	}
}

// parseEnvMap parses an environment variable map from untyped input.
func parseEnvMap(v any, field string) (map[string]string, error) {
	switch e := v.(type) {
	case map[string]any:
		env := make(map[string]string, len(e))
		for k, val := range e {
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%s]: expected string value, got %T", field, k, val)
			}
			env[k] = s
		}
		return env, nil
	case map[string]string:
		return e, nil
	default:
		return nil, fmt.Errorf("%s: expected object, got %T", field, v)
	}
}

// parseStringSlice parses a string slice from untyped input.
func parseStringSlice(v any, field string) ([]string, error) {
	switch s := v.(type) {
	case []any:
		result := make([]string, 0, len(s))
		for i, elem := range s {
			str, ok := elem.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d]: expected string, got %T", field, i, elem)
			}
			if trimmed := strings.TrimSpace(str); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result, nil
	case []string:
		result := make([]string, 0, len(s))
		for _, str := range s {
			if trimmed := strings.TrimSpace(str); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%s: expected array, got %T", field, v)
	}
}

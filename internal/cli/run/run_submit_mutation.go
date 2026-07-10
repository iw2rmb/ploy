package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type buildGateForcedOverrides struct {
	Pre  *contracts.BuildGateStackConfig
	Post *contracts.BuildGateStackConfig
}

func (o buildGateForcedOverrides) hasAny() bool {
	return o.Pre != nil || o.Post != nil
}

func applyStepEnvOverrides(spec json.RawMessage, overrides map[string][]string) (json.RawMessage, error) {
	var specMap map[string]any
	if err := json.Unmarshal(spec, &specMap); err != nil {
		return nil, fmt.Errorf("run submit: parse spec for env overrides: %w", err)
	}
	rawSteps, ok := specMap["steps"].([]any)
	if !ok {
		return nil, errors.New("run submit: spec steps must be an array for env overrides")
	}
	stepsByName := make(map[string]map[string]any, len(rawSteps))
	for i, rawStep := range rawSteps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("run submit: steps[%d] must be an object for env overrides", i)
		}
		name, _ := step["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := stepsByName[name]; exists {
			return nil, fmt.Errorf("run submit: step name %q is not unique", name)
		}
		stepsByName[name] = step
	}
	for stepName, assignments := range overrides {
		step, ok := stepsByName[stepName]
		if !ok {
			return nil, fmt.Errorf("run submit: step %q not found for env override", stepName)
		}
		envs, err := stepEnvMap(step, stepName)
		if err != nil {
			return nil, err
		}
		for _, assignment := range assignments {
			key, value, err := splitStepEnvAssignment(assignment)
			if err != nil {
				return nil, err
			}
			envs[key] = value
		}
		step["envs"] = envs
	}
	mutated, err := json.Marshal(specMap)
	if err != nil {
		return nil, fmt.Errorf("run submit: marshal env-overridden spec: %w", err)
	}
	if _, err := contracts.ParseMigSpecJSON(mutated); err != nil {
		return nil, fmt.Errorf("run submit: validate env-overridden spec: %w", err)
	}
	return json.RawMessage(mutated), nil
}

func stepEnvMap(step map[string]any, stepName string) (map[string]any, error) {
	raw, ok := step["envs"]
	if !ok || raw == nil {
		return make(map[string]any), nil
	}
	envs, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("run submit: step %q envs must be an object", stepName)
	}
	return envs, nil
}

func splitStepEnvAssignment(raw string) (string, string, error) {
	idx := strings.Index(raw, "=")
	if idx < 0 {
		return "", "", fmt.Errorf("run submit: env override %q must be KEY=VALUE", raw)
	}
	key := strings.TrimSpace(raw[:idx])
	if key == "" {
		return "", "", fmt.Errorf("run submit: env override %q has an empty key", raw)
	}
	return key, raw[idx+1:], nil
}

func parseBuildGateForcedFlags(
	globalChanged bool,
	globalValue string,
	preChanged bool,
	preValue string,
	postChanged bool,
	postValue string,
) (buildGateForcedOverrides, error) {
	if globalChanged && (preChanged || postChanged) {
		return buildGateForcedOverrides{}, errors.New("--build-gate-forced cannot be combined with --build-gate-forced-pre or --build-gate-forced-post")
	}

	var out buildGateForcedOverrides
	if globalChanged {
		stack, err := parseBuildGateForcedValue("--build-gate-forced", globalValue)
		if err != nil {
			return buildGateForcedOverrides{}, err
		}
		out.Pre = stack
		out.Post = cloneBuildGateForcedStack(stack)
		return out, nil
	}
	if preChanged {
		stack, err := parseBuildGateForcedValue("--build-gate-forced-pre", preValue)
		if err != nil {
			return buildGateForcedOverrides{}, err
		}
		out.Pre = stack
	}
	if postChanged {
		stack, err := parseBuildGateForcedValue("--build-gate-forced-post", postValue)
		if err != nil {
			return buildGateForcedOverrides{}, err
		}
		out.Post = stack
	}
	return out, nil
}

func parseBuildGateForcedValue(flagName string, raw string) (*contracts.BuildGateStackConfig, error) {
	raw = strings.TrimSpace(raw)
	syntaxErr := func() (*contracts.BuildGateStackConfig, error) {
		return nil, fmt.Errorf("invalid %s value %q: expected <lang>@<release>[/<tool>]", flagName, raw)
	}
	if strings.Count(raw, "@") != 1 {
		return syntaxErr()
	}
	language, releaseAndTool, _ := strings.Cut(raw, "@")
	if strings.Count(releaseAndTool, "/") > 1 {
		return syntaxErr()
	}
	release, tool, hasTool := strings.Cut(releaseAndTool, "/")
	language = strings.TrimSpace(language)
	release = strings.TrimSpace(release)
	tool = strings.TrimSpace(tool)
	if language == "" || release == "" || (hasTool && tool == "") {
		return syntaxErr()
	}
	return &contracts.BuildGateStackConfig{
		Mode:     contracts.BuildGateStackModeForced,
		Language: language,
		Tool:     tool,
		Release:  release,
	}, nil
}

func cloneBuildGateForcedStack(stack *contracts.BuildGateStackConfig) *contracts.BuildGateStackConfig {
	if stack == nil {
		return nil
	}
	clone := *stack
	return &clone
}

func applyBuildGateForcedOverrides(spec json.RawMessage, overrides buildGateForcedOverrides) (json.RawMessage, error) {
	var specMap map[string]any
	if err := json.Unmarshal(spec, &specMap); err != nil {
		return nil, fmt.Errorf("run submit: parse spec for build gate override: %w", err)
	}

	buildGate, err := objectField(specMap, "build_gate", "build_gate")
	if err != nil {
		return nil, err
	}
	buildGate["disabled"] = false
	if overrides.Pre != nil {
		if err := applyBuildGateForcedPhaseOverride(buildGate, "pre", overrides.Pre); err != nil {
			return nil, err
		}
	}
	if overrides.Post != nil {
		if err := applyBuildGateForcedPhaseOverride(buildGate, "post", overrides.Post); err != nil {
			return nil, err
		}
	}
	specMap["build_gate"] = buildGate

	mutated, err := json.Marshal(specMap)
	if err != nil {
		return nil, fmt.Errorf("run submit: marshal build-gate-overridden spec: %w", err)
	}
	if _, err := contracts.ParseMigSpecJSON(mutated); err != nil {
		return nil, fmt.Errorf("run submit: validate build-gate-overridden spec: %w", err)
	}
	return json.RawMessage(mutated), nil
}

func applyBuildGateForcedPhaseOverride(buildGate map[string]any, phaseName string, stack *contracts.BuildGateStackConfig) error {
	phase, err := objectField(buildGate, phaseName, "build_gate."+phaseName)
	if err != nil {
		return err
	}
	stackMap := map[string]any{
		"mode":     string(contracts.BuildGateStackModeForced),
		"language": stack.Language,
		"release":  stack.Release,
	}
	if strings.TrimSpace(stack.Tool) != "" {
		stackMap["tool"] = stack.Tool
	}
	phase["stack"] = stackMap
	buildGate[phaseName] = phase
	return nil
}

func objectField(parent map[string]any, field string, path string) (map[string]any, error) {
	raw, ok := parent[field]
	if !ok || raw == nil {
		return make(map[string]any), nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("run submit: %s must be an object for build gate forced override", path)
	}
	return object, nil
}

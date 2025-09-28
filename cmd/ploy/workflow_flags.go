package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

type stageOverrideFlag struct {
	values []string
}

// String returns the joined representation for the stage overrides flag.
func (f *stageOverrideFlag) String() string {
	return strings.Join(f.values, ",")
}

// Set appends a stage override value while validating empties.
func (f *stageOverrideFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("aster-step value cannot be empty")
	}
	f.values = append(f.values, trimmed)
	return nil
}

// parseStageOverrides interprets --aster-step arguments into runner overrides.
func parseStageOverrides(values []string) (map[string]runner.AsterStageOverride, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]runner.AsterStageOverride)
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --aster-step value: %s", value)
		}
		stage := strings.ToLower(strings.TrimSpace(parts[0]))
		if stage == "" {
			return nil, fmt.Errorf("invalid --aster-step value: stage is required (%s)", value)
		}
		payload := strings.TrimSpace(parts[1])
		override := result[stage]
		if strings.EqualFold(payload, "off") {
			override.Disable = true
			override.ExtraToggles = nil
			result[stage] = override
			continue
		}
		toggles := splitToggles(payload)
		if len(toggles) == 0 {
			return nil, fmt.Errorf("invalid --aster-step toggles for stage %s", stage)
		}
		override.Disable = false
		override.ExtraToggles = append(override.ExtraToggles, toggles...)
		result[stage] = override
	}
	return result, nil
}

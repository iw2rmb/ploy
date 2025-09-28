package mods

import (
	"strings"
	"time"
)

func applyDefaults(opts Options) Options {
	result := opts
	if strings.TrimSpace(result.PlanLane) == "" {
		result.PlanLane = defaultPlanLane
	}
	if strings.TrimSpace(result.OpenRewriteLane) == "" {
		result.OpenRewriteLane = defaultOpenRewriteLane
	}
	if strings.TrimSpace(result.LLMPlanLane) == "" {
		result.LLMPlanLane = defaultLLMPlanLane
	}
	if strings.TrimSpace(result.LLMExecLane) == "" {
		result.LLMExecLane = defaultLLMExecLane
	}
	if strings.TrimSpace(result.HumanLane) == "" {
		result.HumanLane = defaultHumanLane
	}
	if result.PlanTimeout < 0 {
		result.PlanTimeout = 0
	}
	if result.MaxParallel < 0 {
		result.MaxParallel = 0
	}
	return result
}

func formatPlanTimeout(timeout time.Duration) string {
	if timeout <= 0 {
		return ""
	}
	if timeout%time.Millisecond == 0 {
		return timeout.String()
	}
	trimmed := timeout.Truncate(time.Millisecond)
	if trimmed <= 0 {
		trimmed = time.Millisecond
	}
	return trimmed.String()
}

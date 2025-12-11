package plan

import "time"

// applyDefaults normalises planner options with expected defaults.
func applyDefaults(opts Options) Options {
	result := opts
	if result.PlanTimeout < 0 {
		result.PlanTimeout = 0
	}
	if result.MaxParallel < 0 {
		result.MaxParallel = 0
	}
	return result
}

// formatPlanTimeout renders a human-friendly timeout string.
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

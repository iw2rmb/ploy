package orchestration

import "context"

// readyNotifierFn publishes allocation readiness events into the platform event fabric.
// Default to no-op; can be overridden by server initialization or tests.
var readyNotifierFn = func(context.Context, string, *AllocationStatus, int) {}

// SetReadyNotifier overrides the notifier used by HealthMonitor when allocations become healthy.
// If fn is nil, resets to no-op.
func SetReadyNotifier(fn func(context.Context, string, *AllocationStatus, int)) {
	if fn == nil {
		readyNotifierFn = func(context.Context, string, *AllocationStatus, int) {}
		return
	}
	readyNotifierFn = fn
}

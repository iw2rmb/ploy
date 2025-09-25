package server

import "context"

// buildStatusPublisherFn emits build status changes into the event fabric.
// Populated during server initialization; defaults to no-op so tests can override.
var buildStatusPublisherFn = func(context.Context, buildStatus) {}

// SetBuildStatusPublisher overrides the build status publisher used by async build helpers.
// Passing nil resets to the default no-op implementation.
func SetBuildStatusPublisher(fn func(context.Context, buildStatus)) {
	if fn == nil {
		buildStatusPublisherFn = func(context.Context, buildStatus) {}
		return
	}
	buildStatusPublisherFn = fn
}

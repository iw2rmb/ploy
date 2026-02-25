package step

import (
	"context"
	"strings"
)

// GateRuntimeImageObserver is a hook that is called once the gate runtime image
// is resolved (after stack detection / stack gate checks), and before the gate
// container is started.
type GateRuntimeImageObserver func(ctx context.Context, image string)

type gateRuntimeImageObserverKey struct{}
type gateContainerLabelsKey struct{}

// WithGateRuntimeImageObserver attaches an observer to the context used for gate execution.
// The observer is called with the resolved runtime image before container execution starts.
func WithGateRuntimeImageObserver(ctx context.Context, obs GateRuntimeImageObserver) context.Context {
	if obs == nil {
		return ctx
	}
	return context.WithValue(ctx, gateRuntimeImageObserverKey{}, obs)
}

// WithGateContainerLabels attaches container labels to gate execution context.
// Labels are copied and merged with existing gate labels in the context.
func WithGateContainerLabels(ctx context.Context, labels map[string]string) context.Context {
	if len(labels) == 0 {
		return ctx
	}
	merged := gateContainerLabels(ctx)
	if merged == nil {
		merged = make(map[string]string, len(labels))
	}
	for key, value := range labels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		merged[key] = value
	}
	if len(merged) == 0 {
		return ctx
	}
	return context.WithValue(ctx, gateContainerLabelsKey{}, merged)
}

func reportGateRuntimeImage(ctx context.Context, image string) {
	if ctx == nil {
		return
	}
	obs, _ := ctx.Value(gateRuntimeImageObserverKey{}).(GateRuntimeImageObserver)
	if obs == nil {
		return
	}
	image = strings.TrimSpace(image)
	if image == "" {
		return
	}
	obs(ctx, image)
}

func gateContainerLabels(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	labels, _ := ctx.Value(gateContainerLabelsKey{}).(map[string]string)
	if len(labels) == 0 {
		return nil
	}
	copied := make(map[string]string, len(labels))
	for key, value := range labels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		copied[key] = value
	}
	if len(copied) == 0 {
		return nil
	}
	return copied
}

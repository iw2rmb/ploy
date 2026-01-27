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

// WithGateRuntimeImageObserver attaches an observer to the context used for gate execution.
// The observer is called with the resolved runtime image before container execution starts.
func WithGateRuntimeImageObserver(ctx context.Context, obs GateRuntimeImageObserver) context.Context {
	if obs == nil {
		return ctx
	}
	return context.WithValue(ctx, gateRuntimeImageObserverKey{}, obs)
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

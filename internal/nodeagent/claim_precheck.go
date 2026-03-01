package nodeagent

import "context"

// preClaimCleanupFunc decides whether a claim attempt should proceed.
// It is invoked before claim-slot acquisition and before the HTTP claim call.
// A nil value is treated as always-proceed (no cleanup needed).
type preClaimCleanupFunc func(ctx context.Context) (bool, error)

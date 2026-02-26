package nodeagent

import "context"

// preClaimCleanup decides whether a claim attempt should proceed.
// It is invoked before claim-slot acquisition and before the HTTP claim call.
type preClaimCleanup interface {
	EnsureCapacity(ctx context.Context) (bool, error)
}

type noopPreClaimCleanup struct{}

func (noopPreClaimCleanup) EnsureCapacity(context.Context) (bool, error) {
	return true, nil
}

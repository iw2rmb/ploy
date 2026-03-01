package step

import (
	"context"
	"fmt"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func runHydrationStage(ctx context.Context, r *Runner, req Request) (types.Duration, error) {
	stageStart := time.Now()
	if r.Workspace != nil {
		if err := r.Workspace.Hydrate(ctx, req.Manifest, req.Workspace); err != nil {
			return 0, fmt.Errorf("workspace hydration failed: %w", err)
		}
	}
	return types.Duration(time.Since(stageStart)), nil
}

func runGateStage(ctx context.Context, r *Runner, req Request, failMsg string) (*contracts.BuildGateStageMetadata, types.Duration, error) {
	stageStart := time.Now()
	gateSpec := req.Manifest.Gate
	if r.Gate == nil || gateSpec == nil || !gateSpec.Enabled {
		return nil, types.Duration(time.Since(stageStart)), nil
	}

	gateMetadata, err := r.Gate.Execute(ctx, gateSpec, req.Workspace)
	if err != nil {
		return nil, types.Duration(time.Since(stageStart)), fmt.Errorf("build gate execution failed: %w", err)
	}

	gatePassed := false
	if gateMetadata != nil && len(gateMetadata.StaticChecks) > 0 {
		gatePassed = gateMetadata.StaticChecks[0].Passed
	}
	if !gatePassed {
		return gateMetadata, types.Duration(time.Since(stageStart)), fmt.Errorf("%w: %s", ErrBuildGateFailed, failMsg)
	}
	return gateMetadata, types.Duration(time.Since(stageStart)), nil
}

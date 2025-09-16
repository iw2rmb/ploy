package mods

import (
	"context"
	"fmt"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
)

// humanStepTimeout returns the configured timeout for a human-step branch, falling back to default.
func humanStepTimeout(branch BranchSpec, def time.Duration) time.Duration {
	if v, ok := branch.Inputs["timeout"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// humanStepBuildError returns a normalized build error message for MR context.
func humanStepBuildError(inputs map[string]interface{}) string {
	if inputs == nil {
		return "Build failure - human intervention required"
	}
	if v, ok := inputs["buildError"].(string); ok && v != "" {
		return v
	}
	return "Build failure - human intervention required"
}

// humanStepPreflight ensures required runner dependencies are present.
func humanStepPreflight(o *fanoutOrchestrator) (provider.GitProvider, BuildCheckerInterface, error) {
	if o.runner == nil {
		return nil, nil, fmt.Errorf("human-step branches requires production runner (not available in test mode)")
	}
	gp := o.runner.GetGitProvider()
	bc := o.runner.GetBuildChecker()
	if gp == nil || bc == nil {
		return nil, nil, fmt.Errorf("human-step branches require GitProvider and BuildChecker")
	}
	return gp, bc, nil
}

// humanStepMakeMRConfig constructs the MR config for human intervention.
func humanStepMakeMRConfig(repoURL, branchID, buildError string) provider.MRConfig {
	return provider.MRConfig{
		RepoURL:      repoURL,
		SourceBranch: fmt.Sprintf("human-intervention-%s", branchID),
		TargetBranch: "main",
		Title:        fmt.Sprintf("Human Intervention Required: %s", branchID),
		Description:  fmt.Sprintf("Build Error:\n```\n%s\n```\n\nPlease fix the build failure and commit your changes to this branch for automated validation.", buildError),
		Labels:       []string{"ploy", "human-intervention"},
	}
}

// humanStepPollForFix polls at the given interval until buildChecker reports success or context is done.
func humanStepPollForFix(ctx context.Context, buildChecker BuildCheckerInterface, appID string, interval time.Duration) (*common.DeployResult, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			cfg := common.DeployConfig{App: appID, Lane: "A", Environment: "dev", ControllerURL: "", Timeout: ResolveDefaultsFromEnv().BuildApplyTimeout}
			res, err := buildChecker.CheckBuild(ctx, cfg)
			if err != nil {
				continue
			}
			if res != nil && res.Success {
				return res, nil
			}
		}
	}
}

package transflow

import (
	"context"

	"github.com/iw2rmb/ploy/internal/cli/common"
)

// runBuildGate prepares DeployConfig and invokes the build checker using repoPath as working dir.
func (r *TransflowRunner) runBuildGate(ctx context.Context, repoPath string) (*common.DeployResult, error) {
	timeout, err := r.config.ParseBuildTimeout()
	if err != nil {
		return nil, err
	}
	appName := GenerateAppName(r.config.ID)
	buildCfg := common.DeployConfig{
		App:         appName,
		Lane:        r.config.Lane,
		Environment: "dev",
		Timeout:     timeout,
		Metadata:    map[string]string{"working_dir": repoPath},
	}
	if r.buildGate != nil {
		return r.buildGate.Check(ctx, buildCfg)
	}
	return r.buildChecker.CheckBuild(ctx, buildCfg)
}

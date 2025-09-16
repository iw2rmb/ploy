package mods

import (
	"context"
	"fmt"
	"time"

	arf "github.com/iw2rmb/ploy/api/arf"
	buildutil "github.com/iw2rmb/ploy/internal/build"
	"github.com/iw2rmb/ploy/internal/cli/common"
)

// runBuildGate prepares DeployConfig and invokes the build checker using repoPath as working dir.
func (r *ModRunner) runBuildGate(ctx context.Context, repoPath string) (*common.DeployResult, error) {
	timeout, err := r.config.ParseBuildTimeout()
	if err != nil {
		return nil, err
	}

	// Pre-build compile gate using ARF build operations (deterministic, repo-local)
	// This ensures E2E scenarios can trigger compile failures predictably (e.g., via profiles)
	// without depending on downstream deployment plumbing.
	// Respect timeout by creating a child context for compilation.
	compileTimeout := timeout
	if compileTimeout <= 0 {
		compileTimeout = 10 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()
	if bo := arf.NewBuildOperations(compileTimeout); bo != nil {
		// Emit preflight compile gate details for observability
		r.emit(ctx, "build", "compile-gate-start", "info", fmt.Sprintf("repo=%s cmd=%s", repoPath, "mvn clean compile -B -DskipTests -Dploy.build.gate=1"))
		if err := bo.ValidateBuild(cctx, repoPath, ""); err != nil {
			msg := err.Error()
			if be, ok := err.(*buildutil.BuildError); ok {
				msg = buildutil.FormatBuildError(be, true, 64*1024)
			}
			return &common.DeployResult{Success: false, Message: msg}, nil
		}
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

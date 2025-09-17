package mods

import (
	"context"
	"fmt"
	"time"

	build "github.com/iw2rmb/ploy/internal/build"
	"github.com/iw2rmb/ploy/internal/cli/common"
)

// runBuildGate prepares DeployConfig and invokes the build checker using repoPath as working dir.
func (r *ModRunner) runBuildGate(ctx context.Context, repoPath string) (*common.DeployResult, error) {
	timeout, err := r.config.ParseBuildTimeout()
	if err != nil {
		return nil, err
	}

	// Pre-build compile gate using unified sandbox build (deterministic, repo-local)
	// This ensures E2E scenarios can trigger compile failures predictably (e.g., via profiles)
	// without depending on downstream deployment plumbing.
	// Respect timeout by creating a child context for compilation.
	compileTimeout := timeout
	if compileTimeout <= 0 {
		compileTimeout = 10 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()
	service := build.NewSandboxService()
	r.emit(ctx, "build", "compile-gate-start", "info", fmt.Sprintf("repo=%s sandbox=1", repoPath))
	appName := GenerateAppName(r.config.ID)
	res, err := service.Run(cctx, build.SandboxRequest{
		RepoPath: repoPath,
		AppName:  appName,
		SHA:      r.config.BaseRef,
		Lane:     r.config.Lane,
		Timeout:  compileTimeout,
		EnvVars:  nil,
		Options:  build.SandboxOptions{},
	})
	if err != nil {
		return nil, fmt.Errorf("sandbox build failed: %w", err)
	}
	if res != nil && !res.Success {
		msg := res.Message
		if len(res.Errors) > 0 {
			first := res.Errors[0]
			msg = fmt.Sprintf("%s (%s:%d)", msg, first.File, first.Line)
		}
		return &common.DeployResult{Success: false, Message: msg}, nil
	}
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

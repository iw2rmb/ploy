package mods

import (
	"context"
	"fmt"
	"strings"
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
	// Emit preflight compile gate details for observability (build system resolved later).
	r.emit(ctx, "build", "compile-gate-start", "info", fmt.Sprintf("repo=%s sandbox=1", repoPath))
	res, err := service.Run(cctx, build.SandboxRequest{RepoPath: repoPath, Timeout: compileTimeout})
	if err != nil {
		return nil, fmt.Errorf("sandbox build failed: %w", err)
	}
	if res != nil && !res.Success {
		var details []string
		for _, e := range res.Errors {
			details = append(details, fmt.Sprintf("%s:%d:%d %s", e.File, e.Line, e.Column, e.Message))
		}
		be := &build.BuildError{
			Type:    defaultString(res.BuildSystem, "sandbox"),
			Message: res.Message,
			Details: strings.Join(details, "\n"),
			Stdout:  res.Stdout,
			Stderr:  res.Stderr,
		}
		msg := build.FormatBuildError(be, true, 64*1024)
		return &common.DeployResult{Success: false, Message: msg}, nil
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

func defaultString(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}

package mods

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
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
		msg := strings.TrimSpace(res.Message)
		if len(res.Errors) > 0 {
			first := res.Errors[0]
			msg = first.Message
			if strings.TrimSpace(first.File) != "" && first.Line > 0 {
				msg = fmt.Sprintf("%s (%s:%d)", msg, first.File, first.Line)
			}
		}
		if msg == "" {
			msg = "sandbox build failed"
		}
		return &common.DeployResult{Success: false, Message: msg}, nil
	}
	controllerURL := strings.TrimSpace(os.Getenv("PLOY_CONTROLLER"))
	buildCfg := common.DeployConfig{
		App:           appName,
		Lane:          r.config.Lane,
		Environment:   "dev",
		Timeout:       timeout,
		Metadata:      map[string]string{"working_dir": repoPath},
		ControllerURL: controllerURL,
		BuildOnly:     true,
	}
	if r.buildGate != nil {
		deployRes, err := r.buildGate.Check(ctx, buildCfg)
		if err == nil && deployRes != nil && !deployRes.Success {
			enrichBuilderLogs(controllerURL, appName, deployRes)
		}
		return deployRes, err
	}
	deployRes, err := r.buildChecker.CheckBuild(ctx, buildCfg)
	if err == nil && deployRes != nil && !deployRes.Success {
		enrichBuilderLogs(controllerURL, appName, deployRes)
	}
	return deployRes, err
}

func enrichBuilderLogs(controllerURL, appName string, res *common.DeployResult) {
	if res == nil {
		return
	}
	if strings.TrimSpace(res.BuilderLogs) != "" {
		return
	}
	if controllerURL == "" || strings.TrimSpace(res.DeploymentID) == "" {
		return
	}
	logURL := fmt.Sprintf("%s/apps/%s/builds/%s/logs?lines=1200", strings.TrimRight(controllerURL, "/"), appName, res.DeploymentID)
	req, err := http.NewRequest("GET", logURL, nil)
	if err != nil {
		return
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	res.BuilderLogs = string(body)
	if res.BuilderLogsKey == "" {
		res.BuilderLogsKey = fmt.Sprintf("build-logs/%s.log", res.DeploymentID)
	}
	if res.BuilderLogsURL == "" {
		res.BuilderLogsURL = logURL
	}
}

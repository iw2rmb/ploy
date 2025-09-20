package mods

import (
	"context"
	"crypto/tls"
	"encoding/json"
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
	if strings.EqualFold(r.config.Lane, "D") {
		if err := ensureDockerfilePair(repoPath); err != nil {
			r.emit(ctx, "build", "dockerfile-pair", "warn", fmt.Sprintf("failed to prepare Dockerfile pair: %v", err))
		}
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
		if deployRes != nil && !deployRes.Success {
			enrichBuilderLogs(controllerURL, appName, deployRes)
		}
		return deployRes, err
	}
	deployRes, err := r.buildChecker.CheckBuild(ctx, buildCfg)
	if deployRes != nil && !deployRes.Success {
		enrichBuilderLogs(controllerURL, appName, deployRes)
	}
	return deployRes, err
}

func enrichBuilderLogs(controllerURL, appName string, res *common.DeployResult) {
	if res == nil {
		return
	}
	if strings.TrimSpace(res.BuilderLogsKey) == "" && strings.TrimSpace(res.DeploymentID) != "" {
		res.BuilderLogsKey = fmt.Sprintf("build-logs/%s.log", strings.TrimSpace(res.DeploymentID))
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
	logs := strings.TrimSpace(string(body))
	if len(logs) == 0 {
		return
	}
	logs = normalizeBuilderLogs(body, logs)
	res.BuilderLogs = logs
	if res.BuilderLogsKey == "" {
		res.BuilderLogsKey = fmt.Sprintf("build-logs/%s.log", res.DeploymentID)
	}
	if res.BuilderLogsURL == "" {
		res.BuilderLogsURL = logURL
	}
	if snippet := builderLogSnippet(logs, 3, 2000); snippet != "" {
		res.Message = appendUniqueLine(res.Message, snippet)
	}
	if note := buildLogsPointerLine(res.BuilderLogsKey, res.BuilderLogsURL); note != "" {
		res.Message = appendUniqueLine(res.Message, note)
	}
}

func normalizeBuilderLogs(raw []byte, fallback string) string {
	trimmed := strings.TrimSpace(fallback)
	if trimmed == "" {
		trimmed = strings.TrimSpace(string(raw))
	}
	if !json.Valid(raw) {
		return trimmed
	}
	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		if v := strings.TrimSpace(plain); v != "" {
			return v
		}
	}
	var payload struct {
		Logs     *string         `json:"logs"`
		Stdout   *string         `json:"stdout"`
		Stderr   *string         `json:"stderr"`
		Lines    json.RawMessage `json:"lines"`
		Messages json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		if payload.Logs != nil {
			if v := strings.TrimSpace(*payload.Logs); v != "" {
				return v
			}
		}
		if payload.Stdout != nil {
			if v := strings.TrimSpace(*payload.Stdout); v != "" {
				return v
			}
		}
		if payload.Stderr != nil {
			if v := strings.TrimSpace(*payload.Stderr); v != "" {
				return v
			}
		}
		if v := decodeLines(payload.Lines); v != "" {
			return v
		}
		if v := decodeLines(payload.Messages); v != "" {
			return v
		}
	}
	return trimmed
}

func decodeLines(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return strings.TrimSpace(strings.Join(arr, "\n"))
	}
	var generic []interface{}
	if err := json.Unmarshal(raw, &generic); err == nil {
		var parts []string
		for _, item := range generic {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					parts = append(parts, s)
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return ""
}

func builderLogSnippet(logs string, maxLines, maxLen int) string {
	lines := strings.Split(logs, "\n")
	var selected []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		selected = append(selected, line)
		if maxLines > 0 && len(selected) >= maxLines {
			break
		}
	}
	if len(selected) == 0 {
		return ""
	}
	snippet := strings.Join(selected, "\n")
	if maxLen > 0 && len(snippet) > maxLen {
		snippet = snippet[:maxLen] + "…"
	}
	return snippet
}

func buildLogsPointerLine(key, url string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	line := fmt.Sprintf("builder logs archived at %s", key)
	if u := strings.TrimSpace(url); u != "" {
		line = fmt.Sprintf("%s (%s)", line, u)
	}
	return line
}

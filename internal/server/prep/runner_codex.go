package prep

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultPrepTimeout           = 60 * time.Minute
	defaultPromptPath            = "design/prep-prompt.md"
	defaultRunnerOutputLimitByte = 64 * 1024
)

var defaultCodexCommand = []string{"codex", "exec", "--json-output", "--non-interactive"}

// CodexRunnerOptions configures non-interactive Codex CLI execution for prep.
type CodexRunnerOptions struct {
	Command       []string
	GitBinary     string
	PromptPath    string
	WorkspaceRoot string
	Timeout       time.Duration
	Logger        *slog.Logger
}

// CodexRunner executes prep attempts via Codex CLI.
type CodexRunner struct {
	command       []string
	gitBinary     string
	promptPath    string
	workspaceRoot string
	timeout       time.Duration
	logger        *slog.Logger
}

// NewCodexRunner constructs a prep runner backed by Codex CLI.
func NewCodexRunner(opts CodexRunnerOptions) *CodexRunner {
	cmd := opts.Command
	if len(cmd) == 0 {
		cmd = append([]string(nil), defaultCodexCommand...)
	} else {
		cmd = append([]string(nil), cmd...)
	}

	gitBinary := strings.TrimSpace(opts.GitBinary)
	if gitBinary == "" {
		gitBinary = "git"
	}

	promptPath := strings.TrimSpace(opts.PromptPath)
	if promptPath == "" {
		promptPath = defaultPromptPath
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultPrepTimeout
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &CodexRunner{
		command:       cmd,
		gitBinary:     gitBinary,
		promptPath:    promptPath,
		workspaceRoot: strings.TrimSpace(opts.WorkspaceRoot),
		timeout:       timeout,
		logger:        logger,
	}
}

// Run clones the repository and executes one non-interactive Codex prep session.
func (r *CodexRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if r == nil {
		return RunResult{}, newRunError(errors.New("nil codex runner"), FailureCodeUnknown, nil, nil)
	}
	if req.Repo.ID.IsZero() {
		return RunResult{}, newRunError(errors.New("repo id is required"), FailureCodeUnknown, nil, nil)
	}
	if strings.TrimSpace(req.Repo.RepoUrl) == "" {
		return RunResult{}, newRunError(errors.New("repo_url is required"), FailureCodeUnknown, nil, nil)
	}
	if len(r.command) == 0 {
		return RunResult{}, newRunError(errors.New("runner command is empty"), FailureCodeCommandNotFound, nil, nil)
	}

	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	baseDir := r.workspaceRoot
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	workDir, err := os.MkdirTemp(baseDir, "ploy-prep-*")
	if err != nil {
		return RunResult{}, newRunError(err, FailureCodeUnknown, nil, nil)
	}
	defer func() {
		if rmErr := os.RemoveAll(workDir); rmErr != nil {
			r.logger.Warn("prep: failed to clean temporary workspace", "path", workDir, "err", rmErr)
		}
	}()

	repoDir := filepath.Join(workDir, "repo")
	baseRef := strings.TrimSpace(req.Repo.BaseRef)
	cloneArgs := []string{"clone", "--depth", "1"}
	if baseRef != "" {
		cloneArgs = append(cloneArgs, "--branch", baseRef, "--single-branch")
	}
	cloneArgs = append(cloneArgs, req.Repo.RepoUrl, repoDir)
	cloneOutput, cloneErr := runCommand(runCtx, "", r.gitBinary, cloneArgs, nil)
	if cloneErr != nil {
		resultJSON := marshalResultJSON(map[string]any{
			"stage":   "clone",
			"command": append([]string{r.gitBinary}, cloneArgs...),
			"output":  trimOutput(cloneOutput),
		})
		return RunResult{}, newRunError(cloneErr, classifyExecError(cloneErr), resultJSON, strPtr("inline://prep/clone"))
	}

	prompt, promptErr := r.loadPrompt()
	if promptErr != nil {
		resultJSON := marshalResultJSON(map[string]any{
			"stage": "prompt",
			"error": promptErr.Error(),
		})
		return RunResult{}, newRunError(promptErr, FailureCodeUnknown, resultJSON, strPtr("inline://prep/prompt"))
	}

	fullPrompt := renderPrompt(prompt, req)
	cmd := append([]string{}, r.command...)
	cmd = append(cmd, fullPrompt)

	codexOutput, codexErr := runCommand(runCtx, repoDir, cmd[0], cmd[1:], []string{
		"PLOY_PREP_REPO_ID=" + req.Repo.ID.String(),
		"PLOY_PREP_REPO_URL=" + req.Repo.RepoUrl,
		"PLOY_PREP_BASE_REF=" + req.Repo.BaseRef,
		"PLOY_PREP_TARGET_REF=" + req.Repo.TargetRef,
	})

	profileJSON, parseErr := extractProfileJSON(codexOutput)
	if parseErr != nil {
		resultJSON := marshalResultJSON(map[string]any{
			"stage":   "codex",
			"command": cmd,
			"output":  trimOutput(codexOutput),
			"error":   parseErr.Error(),
		})
		code := FailureCodeUnknown
		if codexErr != nil {
			code = classifyExecError(codexErr)
		}
		return RunResult{}, newRunError(parseErr, code, resultJSON, strPtr("inline://prep/codex"))
	}

	if codexErr != nil {
		resultJSON := marshalResultJSON(map[string]any{
			"stage":   "codex",
			"command": cmd,
			"output":  trimOutput(codexOutput),
			"error":   codexErr.Error(),
		})
		return RunResult{}, newRunError(codexErr, classifyExecError(codexErr), resultJSON, strPtr("inline://prep/codex"))
	}

	resultJSON := marshalResultJSON(map[string]any{
		"stage":   "codex",
		"command": cmd,
		"output":  trimOutput(codexOutput),
	})

	return RunResult{
		ProfileJSON: profileJSON,
		ResultJSON:  resultJSON,
		LogsRef:     strPtr("inline://prep/codex"),
	}, nil
}

func (r *CodexRunner) loadPrompt() (string, error) {
	promptBytes, err := os.ReadFile(r.promptPath)
	if err == nil {
		return string(promptBytes), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		r.logger.Warn("prep: prompt file not found; using built-in prompt", "path", r.promptPath)
		return builtinPromptBody, nil
	}
	return "", err
}

func renderPrompt(prompt string, req RunRequest) string {
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\nRuntime metadata:\n")
	b.WriteString("repo_id: ")
	b.WriteString(req.Repo.ID.String())
	b.WriteString("\nrepo_url: ")
	b.WriteString(req.Repo.RepoUrl)
	b.WriteString("\nbase_ref: ")
	b.WriteString(req.Repo.BaseRef)
	b.WriteString("\ntarget_ref: ")
	b.WriteString(req.Repo.TargetRef)
	b.WriteString("\nattempt: ")
	b.WriteString(fmt.Sprintf("%d", req.Attempt))
	return b.String()
}

func runCommand(ctx context.Context, dir, name string, args []string, extraEnv []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return output, ctx.Err()
		}
		return output, err
	}
	return output, nil
}

func classifyExecError(err error) string {
	if err == nil {
		return FailureCodeUnknown
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureCodeTimeout
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		if errors.Is(execErr.Err, exec.ErrNotFound) {
			return FailureCodeCommandNotFound
		}
		return FailureCodeUnknown
	}
	if errors.Is(err, exec.ErrNotFound) {
		return FailureCodeCommandNotFound
	}
	return FailureCodeUnknown
}

func extractProfileJSON(output []byte) ([]byte, error) {
	start := bytes.IndexByte(output, '{')
	if start < 0 {
		return nil, fmt.Errorf("runner output does not contain JSON object")
	}
	end := bytes.LastIndexByte(output, '}')
	if end <= start {
		return nil, fmt.Errorf("runner output does not contain complete JSON object")
	}
	candidate := bytes.TrimSpace(output[start : end+1])
	if !json.Valid(candidate) {
		return nil, fmt.Errorf("runner output JSON is invalid")
	}
	return candidate, nil
}

func trimOutput(raw []byte) string {
	if len(raw) <= defaultRunnerOutputLimitByte {
		return string(raw)
	}
	return string(raw[:defaultRunnerOutputLimitByte])
}

func marshalResultJSON(payload map[string]any) []byte {
	b, err := json.Marshal(payload)
	if err != nil {
		return []byte(`{"error":"failed to marshal prep result"}`)
	}
	return b
}

func strPtr(v string) *string {
	return &v
}

const builtinPromptBody = `You are running in non-interactive prep mode for repository build readiness.

Goal:
Find reproducible settings for this repository in strict priority order:
1. Build
2. Unit tests
3. All tests

Constraints:
- Do not modify repository source code.
- Do not ask user questions.
- Output JSON only and ensure it validates against prep profile schema v1.`

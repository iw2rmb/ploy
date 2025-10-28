package shift

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
)

// CommandResult captures stdout/stderr emitted by the SHIFT CLI invocation.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// CommandRunner executes the SHIFT CLI.
type CommandRunner interface {
	Run(ctx context.Context, cmd []string, env map[string]string, dir string) (CommandResult, error)
}

// Options configures the SHIFT sandbox executor.
type Options struct {
	Runner CommandRunner
	Binary string
}

const defaultBinary = "shift"

// NewExecutor constructs a sandbox executor backed by the SHIFT CLI.
func NewExecutor(opts Options) (buildgate.SandboxExecutor, error) {
	runner := opts.Runner
	if runner == nil {
		runner = execRunner{}
	}
	binary := strings.TrimSpace(opts.Binary)
	if binary == "" {
		if configured := strings.TrimSpace(os.Getenv("PLOY_SHIFT_BINARY")); configured != "" {
			binary = configured
		} else {
			binary = defaultBinary
		}
	}
	return &executor{runner: runner, binary: binary}, nil
}

type executor struct {
	runner CommandRunner
	binary string
}

func (e *executor) Execute(ctx context.Context, spec buildgate.SandboxSpec) (buildgate.SandboxBuildResult, error) {
	workspace := strings.TrimSpace(spec.Workspace)
	if workspace == "" {
		return buildgate.SandboxBuildResult{}, fmt.Errorf("shift: workspace path required")
	}

	args := []string{"run", "--path", workspace, "--output", "json"}
	if profile := strings.TrimSpace(spec.Env["PLOY_SHIFT_PROFILE"]); profile != "" {
		args = append(args, "--lane", profile)
	}

	env := mergeEnv(spec.Env)
	result, runErr := e.runner.Run(ctx, append([]string{e.binary}, args...), env, workspace)
	logDigest := digest(result.Stdout, result.Stderr)

	if runErr != nil {
		return buildgate.SandboxBuildResult{
			Success:       false,
			CacheHit:      false,
			LogDigest:     logDigest,
			FailureReason: "execution",
			FailureDetail: strings.TrimSpace(runErr.Error()),
		}, runErr
	}

	buildResult := buildgate.SandboxBuildResult{
		CacheHit:  false,
		LogDigest: logDigest,
	}

	rawSummary := strings.TrimSpace(result.Stdout)
	var summary *executionSummary
	if rawSummary != "" {
		if parsed, parseErr := parseExecutionSummary(rawSummary); parseErr == nil {
			summary = &parsed
			buildResult.Metadata = metadataFromSummary(parsed)
		}
		if len(buildResult.Metadata.LogFindings) == 0 && strings.TrimSpace(result.Stderr) != "" {
			buildResult.Metadata.LogFindings = append(buildResult.Metadata.LogFindings, buildgate.LogFinding{
				Code:     "shift.stderr",
				Severity: "error",
				Message:  firstLine(result.Stderr),
				Evidence: strings.TrimSpace(result.Stderr),
			})
		}
		buildResult.Report = []byte(rawSummary)
	}

	success := result.ExitCode == 0
	if summary != nil {
		if !strings.EqualFold(strings.TrimSpace(summary.Status), "success") {
			success = false
			if reason := strings.TrimSpace(summary.Status); reason != "" {
				buildResult.FailureReason = reason
			}
			if detail := failureDetailFromSummary(*summary, result.Stderr, result.ExitCode); detail != "" {
				buildResult.FailureDetail = detail
			}
		}
	}

	if !success {
		if buildResult.FailureReason == "" {
			buildResult.FailureReason = "exit_code"
		}
		if buildResult.FailureDetail == "" {
			detail := strings.TrimSpace(result.Stderr)
			if detail == "" {
				detail = strings.TrimSpace(result.Stdout)
			}
			if detail == "" {
				detail = fmt.Sprintf("shift CLI exited with code %d", result.ExitCode)
			} else {
				detail = fmt.Sprintf("shift CLI exited with code %d: %s", result.ExitCode, firstLine(detail))
			}
			buildResult.FailureDetail = detail
		}
	} else {
		buildResult.FailureReason = ""
		buildResult.FailureDetail = ""
	}
	buildResult.Success = success

	return buildResult, nil
}

func mergeEnv(specEnv map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			merged[parts[0]] = parts[1]
		}
	}
	for k, v := range specEnv {
		merged[k] = v
	}
	return merged
}

func digest(stdout, stderr string) string {
	combined := strings.TrimSpace(stdout + stderr)
	if combined == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(stdout + stderr))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func metadataFromSummary(summary executionSummary) buildgate.Metadata {
	var findings []buildgate.LogFinding
	for _, diag := range summary.Diagnostics {
		message := strings.TrimSpace(diag.Message)
		if message == "" {
			message = strings.TrimSpace(diag.Code)
		}
		findings = append(findings, buildgate.LogFinding{
			Code:     strings.TrimSpace(diag.Code),
			Severity: strings.ToLower(strings.TrimSpace(diag.Severity)),
			Message:  message,
			Evidence: strings.TrimSpace(diag.Path),
		})
	}

	status := strings.TrimSpace(summary.Status)
	severity := "info"
	if !strings.EqualFold(status, "success") && status != "" {
		severity = "error"
	}
	duration := ""
	if summary.DurationMs > 0 {
		duration = (time.Duration(summary.DurationMs) * time.Millisecond).Round(time.Millisecond).String()
	}
	evidenceParts := []string{}
	if summary.Lane != "" {
		evidenceParts = append(evidenceParts, fmt.Sprintf("lane=%s", summary.Lane))
	}
	if summary.Orchestrator != "" {
		evidenceParts = append(evidenceParts, fmt.Sprintf("executor=%s", summary.Orchestrator))
	}
	if duration != "" {
		evidenceParts = append(evidenceParts, fmt.Sprintf("duration=%s", duration))
	}
	if summary.ExitCode != 0 {
		evidenceParts = append(evidenceParts, fmt.Sprintf("exit_code=%d", summary.ExitCode))
	}
	if summary.Workspace != "" {
		evidenceParts = append(evidenceParts, fmt.Sprintf("workspace=%s", summary.Workspace))
	}
	findings = append(findings, buildgate.LogFinding{
		Code:     "shift.summary",
		Severity: severity,
		Message:  fmt.Sprintf("shift run status %s", defaultString(status, "unknown")),
		Evidence: strings.Join(evidenceParts, " "),
	})

	return buildgate.Metadata{
		LogFindings: findings,
	}
}

func failureDetailFromSummary(summary executionSummary, stderr string, exitCode int) string {
	for _, diag := range summary.Diagnostics {
		if strings.TrimSpace(diag.Message) == "" {
			continue
		}
		detail := strings.TrimSpace(diag.Message)
		if path := strings.TrimSpace(diag.Path); path != "" {
			detail = fmt.Sprintf("%s (%s)", detail, path)
		}
		return detail
	}
	trimmed := strings.TrimSpace(stderr)
	if trimmed != "" {
		return trimmed
	}
	if status := strings.TrimSpace(summary.Status); status != "" {
		return fmt.Sprintf("shift status %s (exit code %d)", status, exitCode)
	}
	return ""
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

type executionSummary struct {
	RunID        string              `json:"run_id"`
	Status       string              `json:"status"`
	Lane         string              `json:"lane"`
	Orchestrator string              `json:"orchestrator"`
	ExitCode     int                 `json:"exit_code"`
	DurationMs   int64               `json:"duration_ms"`
	Workspace    string              `json:"workspace"`
	Diagnostics  []diagnosticPayload `json:"diagnostics"`
}

type diagnosticPayload struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path"`
}

func parseExecutionSummary(raw string) (executionSummary, error) {
	if strings.TrimSpace(raw) == "" {
		return executionSummary{}, fmt.Errorf("shift: empty summary")
	}
	var summary executionSummary
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return executionSummary{}, fmt.Errorf("shift: parse execution summary: %w", err)
	}
	return summary, nil
}

func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, cmd []string, env map[string]string, dir string) (CommandResult, error) {
	if len(cmd) == 0 {
		return CommandResult{}, errors.New("shift: command missing")
	}
	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	if dir != "" {
		command.Dir = dir
	}
	command.Env = flattenEnv(env)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func flattenEnv(env map[string]string) []string {
	if len(env) == 0 {
		return os.Environ()
	}
	merged := mergeEnv(env)
	entries := make([]string, 0, len(merged))
	for k, v := range merged {
		entries = append(entries, fmt.Sprintf("%s=%s", k, v))
	}
	return entries
}

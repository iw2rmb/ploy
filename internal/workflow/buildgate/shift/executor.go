package shift

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
		Success:   result.ExitCode == 0,
		CacheHit:  false,
		LogDigest: logDigest,
	}

	if result.ExitCode != 0 {
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(result.Stdout)
		}
		if detail == "" {
			detail = fmt.Sprintf("shift CLI exited with code %d", result.ExitCode)
		} else {
			detail = fmt.Sprintf("shift CLI exited with code %d: %s", result.ExitCode, firstLine(detail))
		}
		buildResult.FailureReason = "exit_code"
		buildResult.FailureDetail = detail
	}

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

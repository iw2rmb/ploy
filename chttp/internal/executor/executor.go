package executor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/chttp/internal/config"
)

// ExecuteRequest represents a CLI command execution request
type ExecuteRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout string   `json:"timeout,omitempty"`
}

// ExecuteResponse represents the response from CLI command execution
type ExecuteResponse struct {
	Success  bool   `json:"success"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
}

// CLIExecutor handles CLI command execution
type CLIExecutor struct {
	config *config.Config
}

// NewCLIExecutor creates a new CLI executor
func NewCLIExecutor(cfg *config.Config) *CLIExecutor {
	return &CLIExecutor{
		config: cfg,
	}
}

// Execute executes a CLI command and returns the result
func (e *CLIExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	// Validate command is allowed
	if !e.config.IsCommandAllowed(req.Command) {
		return &ExecuteResponse{
			Success:  false,
			Stderr:   fmt.Sprintf("Command '%s' is not allowed", req.Command),
			ExitCode: 1,
		}, fmt.Errorf("command not allowed: %s", req.Command)
	}

	// Parse timeout
	timeout := e.config.Commands.DefaultTimeout
	if req.Timeout != "" {
		if parsedTimeout, err := time.ParseDuration(req.Timeout); err == nil {
			timeout = parsedTimeout
		}
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Record start time
	start := time.Now()

	// Create command
	cmd := exec.CommandContext(execCtx, req.Command, req.Args...)

	// Execute command and capture output
	stdout, stderr, exitCode, err := e.executeCommand(cmd)
	duration := time.Since(start)

	response := &ExecuteResponse{
		Success:  err == nil && exitCode == 0,
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Duration: duration.String(),
	}

	return response, err
}

// executeCommand executes the command and captures all output
func (e *CLIExecutor) executeCommand(cmd *exec.Cmd) (string, string, int, error) {
	// Capture stdout and stderr
	stdoutBytes, err := cmd.Output()
	stdout := string(stdoutBytes)
	stderr := ""
	exitCode := 0

	if err != nil {
		// Handle exit errors to get stderr and exit code
		if exitError, ok := err.(*exec.ExitError); ok {
			stderr = string(exitError.Stderr)
			exitCode = exitError.ExitCode()
		} else {
			// Other errors (e.g., command not found, timeout)
			stderr = err.Error()
			exitCode = -1
		}
	}

	// Clean output strings
	stdout = strings.TrimSpace(stdout)
	stderr = strings.TrimSpace(stderr)

	return stdout, stderr, exitCode, err
}

// ValidateRequest validates an execution request
func (e *CLIExecutor) ValidateRequest(req ExecuteRequest) error {
	if req.Command == "" {
		return fmt.Errorf("command is required")
	}

	if !e.config.IsCommandAllowed(req.Command) {
		return fmt.Errorf("command '%s' is not allowed", req.Command)
	}

	// Validate timeout format if provided
	if req.Timeout != "" {
		if _, err := time.ParseDuration(req.Timeout); err != nil {
			return fmt.Errorf("invalid timeout format: %s", req.Timeout)
		}
	}

	return nil
}
package buildgate

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GoVetAdapter executes `go vet` for Go repositories and normalises the
// diagnostics into StaticCheckFailure entries.
type GoVetAdapter struct {
	runner   commandRunner
	workDir  string
	env      []string
	goBinary string
}

// GoVetOption configures the behaviour of a GoVetAdapter instance.
type GoVetOption func(*GoVetAdapter)

// NewGoVetAdapter creates a Go static check adapter backed by `go vet`.
func NewGoVetAdapter(workDir string, opts ...GoVetOption) *GoVetAdapter {
	adapter := &GoVetAdapter{
		runner:   execCommandRunner{},
		workDir:  workDir,
		env:      os.Environ(),
		goBinary: "go",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter
}

// WithGoVetEnv overrides the environment used when invoking `go vet`.
func WithGoVetEnv(env []string) GoVetOption {
	copied := append([]string(nil), env...)
	return func(adapter *GoVetAdapter) {
		adapter.env = copied
	}
}

// WithGoVetBinary overrides the go binary used by the adapter.
func WithGoVetBinary(binary string) GoVetOption {
	trimmed := strings.TrimSpace(binary)
	return func(adapter *GoVetAdapter) {
		if trimmed != "" {
			adapter.goBinary = trimmed
		}
	}
}

// withGoVetCommandRunner injects a custom command runner. It is intended for
// tests and remains unexported.
func withGoVetCommandRunner(runner commandRunner) GoVetOption {
	return func(adapter *GoVetAdapter) {
		if runner != nil {
			adapter.runner = runner
		}
	}
}

// Metadata exposes the adapter information required by the registry.
func (a *GoVetAdapter) Metadata() StaticCheckAdapterMetadata {
	return StaticCheckAdapterMetadata{
		Language:        "golang",
		Tool:            "go vet",
		DefaultSeverity: SeverityError,
	}
}

// Run executes `go vet` using the configured runner and working directory.
func (a *GoVetAdapter) Run(ctx context.Context, req StaticCheckRequest) (StaticCheckResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	packages := parseGoVetPackages(req.Options)
	args := []string{"vet"}
	if tags := strings.TrimSpace(req.Options["tags"]); tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, packages...)

	output, runErr := a.runner.Run(ctx, a.workDir, a.env, a.goBinary, args...)
	failures, parseErr := parseGoVetOutput(output.stdout, output.stderr)
	if parseErr != nil {
		return StaticCheckResult{}, parseErr
	}
	if runErr != nil && len(failures) == 0 {
		return StaticCheckResult{}, fmt.Errorf("go vet: %w", runErr)
	}
	return StaticCheckResult{Failures: failures}, nil
}

func parseGoVetPackages(options map[string]string) []string {
	raw := strings.TrimSpace(options["packages"])
	if raw == "" {
		return []string{"./..."}
	}
	normalized := strings.NewReplacer(",", " ", "\n", " ", "\t", " ").Replace(raw)
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return []string{"./..."}
	}
	return fields
}

func parseGoVetOutput(stdout, stderr string) ([]StaticCheckFailure, error) {
	var buf bytes.Buffer
	if strings.TrimSpace(stdout) != "" {
		buf.WriteString(stdout)
		if !strings.HasSuffix(stdout, "\n") {
			buf.WriteByte('\n')
		}
	}
	if strings.TrimSpace(stderr) != "" {
		buf.WriteString(stderr)
	}

	scanner := bufio.NewScanner(strings.NewReader(buf.String()))
	var failures []StaticCheckFailure
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		failure, ok := parseGoVetLine(line)
		if ok {
			failures = append(failures, failure)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse go vet output: %w", err)
	}
	return failures, nil
}

func parseGoVetLine(line string) (StaticCheckFailure, bool) {
	parts := strings.SplitN(line, ":", 4)
	if len(parts) < 3 {
		return StaticCheckFailure{}, false
	}

	file := filepath.Clean(strings.TrimSpace(parts[0]))
	lineNumber, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return StaticCheckFailure{}, false
	}

	message := ""
	column := 0
	if len(parts) == 3 {
		message = strings.TrimSpace(parts[2])
	} else {
		columnValue := strings.TrimSpace(parts[2])
		if parsedColumn, colErr := strconv.Atoi(columnValue); colErr == nil {
			column = parsedColumn
			message = strings.TrimSpace(parts[3])
		} else {
			message = strings.TrimSpace(parts[2] + ":" + parts[3])
		}
	}

	return StaticCheckFailure{
		RuleID:   "govet",
		File:     file,
		Line:     clampNonNegative(lineNumber),
		Column:   clampNonNegative(column),
		Severity: string(SeverityError),
		Message:  message,
	}, true
}

type commandRunner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) (commandOutput, error)
}

type commandOutput struct {
	stdout string
	stderr string
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (commandOutput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append([]string{}, env...)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return commandOutput{stdout: stdout.String(), stderr: stderr.String()}, err
}

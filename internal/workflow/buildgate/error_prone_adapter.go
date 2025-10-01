package buildgate

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ErrorProneAdapter executes Java Error Prone analysis and normalises diagnostics into StaticCheckFailure entries.
type ErrorProneAdapter struct {
	runner     commandRunner
	workDir    string
	env        []string
	javaBinary string
}

// ErrorProneOption configures the behaviour of an ErrorProneAdapter instance.
type ErrorProneOption func(*ErrorProneAdapter)

// NewErrorProneAdapter constructs a Java static check adapter backed by Error Prone.
func NewErrorProneAdapter(workDir string, opts ...ErrorProneOption) *ErrorProneAdapter {
	adapter := &ErrorProneAdapter{
		runner:     execCommandRunner{},
		workDir:    workDir,
		env:        os.Environ(),
		javaBinary: "javac",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter
}

// WithErrorProneEnv overrides the environment used when invoking Error Prone.
func WithErrorProneEnv(env []string) ErrorProneOption {
	copied := append([]string(nil), env...)
	return func(adapter *ErrorProneAdapter) {
		adapter.env = copied
	}
}

// WithErrorProneBinary overrides the Java binary used to launch Error Prone.
func WithErrorProneBinary(binary string) ErrorProneOption {
	trimmed := strings.TrimSpace(binary)
	return func(adapter *ErrorProneAdapter) {
		if trimmed != "" {
			adapter.javaBinary = trimmed
		}
	}
}

// withErrorProneCommandRunner injects a custom command runner for testing.
func withErrorProneCommandRunner(runner commandRunner) ErrorProneOption {
	return func(adapter *ErrorProneAdapter) {
		if runner != nil {
			adapter.runner = runner
		}
	}
}

// Metadata exposes the adapter information required by the registry.
func (a *ErrorProneAdapter) Metadata() StaticCheckAdapterMetadata {
	return StaticCheckAdapterMetadata{
		Language:        "java",
		Tool:            "Error Prone",
		DefaultSeverity: SeverityError,
	}
}

// Run executes Error Prone using the configured runner and translates diagnostics into StaticCheckResult failures.
func (a *ErrorProneAdapter) Run(ctx context.Context, req StaticCheckRequest) (StaticCheckResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	args := []string{"-Xplugin:ErrorProne"}
	if classpath := strings.TrimSpace(req.Options["classpath"]); classpath != "" {
		args = append(args, "-classpath", classpath)
	}
	if flags := parseErrorProneFlags(req.Options["flags"]); len(flags) > 0 {
		args = append(args, flags...)
	}
	targets := parseErrorProneTargets(req.Options["targets"])
	if len(targets) == 0 {
		targets = []string{"."}
	}
	args = append(args, targets...)

	output, runErr := a.runner.Run(ctx, a.workDir, a.env, a.javaBinary, args...)
	failures, parseErr := parseErrorProneDiagnostics(output.stdout, output.stderr)
	if parseErr != nil {
		return StaticCheckResult{}, parseErr
	}
	if runErr != nil && len(failures) == 0 {
		return StaticCheckResult{}, fmt.Errorf("error prone: %w", runErr)
	}
	return StaticCheckResult{Failures: failures}, nil
}

var errorPronePattern = regexp.MustCompile(`^(.+?):(\d+)(?::(\d+))?:\s*(error|warning):\s*\[([^\]]+)\]\s*(.*)$`)

// parseErrorProneDiagnostics converts Error Prone stdout/stderr into structured failures.
func parseErrorProneDiagnostics(stdout, stderr string) ([]StaticCheckFailure, error) {
	var buffer bytes.Buffer
	if strings.TrimSpace(stdout) != "" {
		buffer.WriteString(stdout)
		if !strings.HasSuffix(stdout, "\n") {
			buffer.WriteByte('\n')
		}
	}
	if strings.TrimSpace(stderr) != "" {
		buffer.WriteString(stderr)
	}

	scanner := bufio.NewScanner(strings.NewReader(buffer.String()))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var failures []StaticCheckFailure
	var current *StaticCheckFailure
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if matches := errorPronePattern.FindStringSubmatch(line); matches != nil {
			if current != nil {
				failures = append(failures, *current)
			}
			file := filepath.Clean(strings.TrimSpace(matches[1]))
			lineNumber, _ := strconv.Atoi(strings.TrimSpace(matches[2]))
			column := 0
			if matches[3] != "" {
				column, _ = strconv.Atoi(strings.TrimSpace(matches[3]))
			}
			severity := normalizeSeverity(matches[4])
			rule := strings.TrimSpace(matches[5])
			message := strings.TrimSpace(matches[6])
			current = &StaticCheckFailure{
				RuleID:   rule,
				File:     file,
				Line:     clampNonNegative(lineNumber),
				Column:   clampNonNegative(column),
				Severity: severity,
				Message:  message,
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if current != nil {
			if current.Message == "" {
				current.Message = trimmed
			} else {
				current.Message = current.Message + "\n" + trimmed
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse error prone output: %w", err)
	}
	if current != nil {
		failures = append(failures, *current)
	}
	return failures, nil
}

// parseErrorProneTargets derives target file arguments for Error Prone invocation.
func parseErrorProneTargets(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	replacer := strings.NewReplacer(",", " ", "\n", " ", "\t", " ")
	fields := strings.Fields(replacer.Replace(trimmed))
	results := make([]string, 0, len(fields))
	for _, field := range fields {
		cleaned := strings.TrimSpace(field)
		if cleaned != "" {
			results = append(results, cleaned)
		}
	}
	return results
}

// parseErrorProneFlags splits manifest-provided flags into individual arguments preserving order.
func parseErrorProneFlags(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	replacer := strings.NewReplacer("\n", " ", "\t", " ")
	fields := strings.Fields(replacer.Replace(trimmed))
	results := make([]string, 0, len(fields))
	for _, field := range fields {
		cleaned := strings.TrimSpace(field)
		if cleaned != "" {
			results = append(results, cleaned)
		}
	}
	return results
}

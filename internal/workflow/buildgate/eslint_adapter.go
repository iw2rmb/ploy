package buildgate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ESLintAdapter executes ESLint analysis and normalises findings into StaticCheckFailure entries.
type ESLintAdapter struct {
	runner       commandRunner
	workDir      string
	env          []string
	eslintBinary string
}

// ESLintOption configures an ESLintAdapter instance behaviour.
type ESLintOption func(*ESLintAdapter)

// NewESLintAdapter constructs a JavaScript static check adapter backed by ESLint.
func NewESLintAdapter(workDir string, opts ...ESLintOption) *ESLintAdapter {
	adapter := &ESLintAdapter{
		runner:       execCommandRunner{},
		workDir:      workDir,
		env:          os.Environ(),
		eslintBinary: "eslint",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter
}

// WithESLintEnv overrides the environment used when invoking ESLint.
func WithESLintEnv(env []string) ESLintOption {
	copied := append([]string(nil), env...)
	return func(adapter *ESLintAdapter) {
		adapter.env = copied
	}
}

// WithESLintBinary overrides the ESLint binary used by the adapter.
func WithESLintBinary(binary string) ESLintOption {
	trimmed := strings.TrimSpace(binary)
	return func(adapter *ESLintAdapter) {
		if trimmed != "" {
			adapter.eslintBinary = trimmed
		}
	}
}

// withESLintCommandRunner injects a custom command runner for tests.
func withESLintCommandRunner(runner commandRunner) ESLintOption {
	return func(adapter *ESLintAdapter) {
		if runner != nil {
			adapter.runner = runner
		}
	}
}

// Metadata exposes the adapter information required by the registry.
func (a *ESLintAdapter) Metadata() StaticCheckAdapterMetadata {
	return StaticCheckAdapterMetadata{
		Language:        "javascript",
		Tool:            "ESLint",
		DefaultSeverity: SeverityError,
	}
}

// Run executes ESLint using the configured runner and translates diagnostics into StaticCheckResult failures.
func (a *ESLintAdapter) Run(ctx context.Context, req StaticCheckRequest) (StaticCheckResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	binary := a.eslintBinary
	var prefix []string
	if override := strings.TrimSpace(req.Options["binary"]); override != "" {
		parts := strings.Fields(override)
		if len(parts) > 0 {
			binary = parts[0]
			if len(parts) > 1 {
				prefix = append(prefix, parts[1:]...)
			}
		}
	}

	baseArgs := []string{"--format", "json", "--no-color", "--no-error-on-unmatched-pattern"}
	if config := strings.TrimSpace(req.Options["config"]); config != "" {
		baseArgs = append(baseArgs, "--config", config)
	}
	for _, override := range parseESLintRuleOverrides(req.Options["rule_overrides"]) {
		baseArgs = append(baseArgs, "--rule", override)
	}
	targets := parseESLintTargets(req.Options["targets"])
	if len(targets) == 0 {
		targets = []string{"."}
	}
	baseArgs = append(baseArgs, targets...)

	args := append(prefix, baseArgs...)

	output, runErr := a.runner.Run(ctx, a.workDir, a.env, binary, args...)
	failures, parseErr := parseESLintDiagnostics(output.stdout)
	if parseErr != nil {
		return StaticCheckResult{}, parseErr
	}
	if runErr != nil && len(failures) == 0 {
		return StaticCheckResult{}, fmt.Errorf("eslint: %w", runErr)
	}
	return StaticCheckResult{Failures: failures}, nil
}

// parseESLintDiagnostics converts ESLint JSON output into structured failures.
func parseESLintDiagnostics(stdout string) ([]StaticCheckFailure, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, nil
	}

	var reports []struct {
		FilePath string `json:"filePath"`
		Messages []struct {
			RuleID   *string `json:"ruleId"`
			Message  string  `json:"message"`
			Line     int     `json:"line"`
			Column   int     `json:"column"`
			Severity int     `json:"severity"`
			Fatal    bool    `json:"fatal"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(trimmed), &reports); err != nil {
		return nil, fmt.Errorf("parse eslint output: %w", err)
	}

	failures := make([]StaticCheckFailure, 0)
	for _, report := range reports {
		filePath := strings.TrimSpace(report.FilePath)
		file := filePath
		if filePath != "" {
			file = filepath.Clean(filePath)
		}
		for _, message := range report.Messages {
			rule := "eslint"
			if message.RuleID != nil {
				if trimmedRule := strings.TrimSpace(*message.RuleID); trimmedRule != "" {
					rule = trimmedRule
				}
			}
			failure := StaticCheckFailure{
				RuleID:   rule,
				File:     file,
				Line:     clampNonNegative(message.Line),
				Column:   clampNonNegative(message.Column),
				Severity: eslintSeverityName(message.Severity, message.Fatal),
				Message:  strings.TrimSpace(message.Message),
			}
			failures = append(failures, failure)
		}
	}
	return failures, nil
}

// eslintSeverityName converts ESLint severities into registry severity names.
func eslintSeverityName(severity int, fatal bool) string {
	if fatal || severity >= 2 {
		return string(SeverityError)
	}
	if severity == 1 {
		return string(SeverityWarning)
	}
	return string(SeverityInfo)
}

// parseESLintTargets derives ESLint target arguments from manifest options.
func parseESLintTargets(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	replacer := strings.NewReplacer(",", " ", "\n", " ", "\t", " ")
	fields := strings.Fields(replacer.Replace(trimmed))
	targets := make([]string, 0, len(fields))
	for _, field := range fields {
		cleaned := strings.TrimSpace(field)
		if cleaned != "" {
			targets = append(targets, cleaned)
		}
	}
	return targets
}

// parseESLintRuleOverrides converts rule overrides into repeated --rule arguments.
func parseESLintRuleOverrides(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	replacer := strings.NewReplacer("\n", " ", "\t", " ", ",", " ")
	fields := strings.Fields(replacer.Replace(trimmed))
	overrides := make([]string, 0, len(fields))
	for _, field := range fields {
		parts := strings.SplitN(field, ":", 2)
		rule := strings.TrimSpace(parts[0])
		if rule == "" {
			continue
		}
		if len(parts) == 1 {
			overrides = append(overrides, rule)
			continue
		}
		severity := strings.ToLower(strings.TrimSpace(parts[1]))
		switch severity {
		case "warning":
			severity = "warn"
		case "errors":
			severity = "error"
		case "warnings":
			severity = "warn"
		}
		overrides = append(overrides, fmt.Sprintf("%s:%s", rule, severity))
	}
	return overrides
}

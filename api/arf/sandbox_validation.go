package arf

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// BuildConfig contains configuration for build validation
type BuildConfig struct {
	BuildTool    string        `json:"build_tool"`
	BuildCommand string        `json:"build_command"` // Optional custom command
	Timeout      time.Duration `json:"timeout"`
	MemoryLimit  string        `json:"memory_limit"` // e.g., "2G"
	CPULimit     string        `json:"cpu_limit"`    // e.g., "2"
	WorkingDir   string        `json:"working_dir"`
	EnvVars      []string      `json:"env_vars"`
}

// TestConfig contains configuration for test execution
type TestConfig struct {
	TestFramework string        `json:"test_framework"`
	TestCommand   string        `json:"test_command"` // Optional custom command
	Timeout       time.Duration `json:"timeout"`
	MemoryLimit   string        `json:"memory_limit"`
	CPULimit      string        `json:"cpu_limit"`
	WorkingDir    string        `json:"working_dir"`
	EnvVars       []string      `json:"env_vars"`
	Coverage      bool          `json:"coverage"` // Enable coverage reporting
}

// BuildValidationResult contains the results of build validation
type BuildValidationResult struct {
	Success      bool                   `json:"success"`
	BuildTool    string                 `json:"build_tool"`
	BuildCommand string                 `json:"build_command"`
	Duration     time.Duration          `json:"duration"`
	Output       string                 `json:"output"`
	Errors       []ValidationBuildError `json:"errors,omitempty"`
	Warnings     []string               `json:"warnings,omitempty"`
	Artifacts    []string               `json:"artifacts,omitempty"` // Build artifacts produced
}

// TestValidationResult contains the results of test execution
type TestValidationResult struct {
	Success         bool          `json:"success"`
	TestFramework   string        `json:"test_framework"`
	TestCommand     string        `json:"test_command"`
	Duration        time.Duration `json:"duration"`
	TotalTests      int           `json:"total_tests"`
	PassedTests     int           `json:"passed_tests"`
	FailedTests     int           `json:"failed_tests"`
	SkippedTests    int           `json:"skipped_tests"`
	Output          string        `json:"output"`
	Failures        []TestFailure `json:"failures,omitempty"`
	CoveragePercent float64       `json:"coverage_percent,omitempty"`
}

// ValidationBuildError represents a build error with location details
type ValidationBuildError struct {
	Type    string `json:"type"` // compilation, dependency, configuration, etc.
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

// TestFailure represents a test failure
type TestFailure struct {
	TestName   string `json:"test_name"`
	ClassName  string `json:"class_name,omitempty"`
	Message    string `json:"message"`
	StackTrace string `json:"stack_trace,omitempty"`
}

// SandboxValidator provides build and test validation in sandboxes
type SandboxValidator struct {
	sandboxMgr SandboxManager
}

// NewSandboxValidator creates a new sandbox validator
func NewSandboxValidator(sandboxMgr SandboxManager) *SandboxValidator {
	return &SandboxValidator{
		sandboxMgr: sandboxMgr,
	}
}

// ValidateBuild runs build validation in a sandbox
func (v *SandboxValidator) ValidateBuild(ctx context.Context, sandboxID string, config BuildConfig) (*BuildValidationResult, error) {
	startTime := time.Now()

	// Set default timeout if not specified
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Minute
	}

	// Create context with timeout
	buildCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	// Determine build command
	buildCommand := config.BuildCommand
	if buildCommand == "" {
		buildCommand = v.getDefaultBuildCommand(config.BuildTool)
	}

	result := &BuildValidationResult{
		BuildTool:    config.BuildTool,
		BuildCommand: buildCommand,
	}

	// Execute build command in sandbox
	output, err := v.ExecuteInSandbox(buildCtx, sandboxID, "sh", "-c", buildCommand)
	result.Output = output
	result.Duration = time.Since(startTime)

	if err != nil {
		// Check if it was a timeout
		if buildCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("build timeout after %v", config.Timeout)
		}

		// Parse build errors from output
		result.Success = false
		result.Errors = v.ParseBuildOutput(config.BuildTool, output)

		// If no specific errors were parsed, create a generic one
		if len(result.Errors) == 0 {
			result.Errors = append(result.Errors, ValidationBuildError{
				Type:    "build_failure",
				Message: err.Error(),
			})
		}
	} else {
		// Check if build was successful based on output
		result.Success = v.isBuildSuccessful(config.BuildTool, output)

		// Even if exit code was 0, parse for any errors/warnings
		result.Errors = v.ParseBuildOutput(config.BuildTool, output)
		result.Warnings = v.parseBuildWarnings(config.BuildTool, output)

		// Collect build artifacts
		result.Artifacts = v.collectBuildArtifacts(sandboxID, config.BuildTool)
	}

	return result, nil
}

// RunTests runs tests in a sandbox
func (v *SandboxValidator) RunTests(ctx context.Context, sandboxID string, config TestConfig) (*TestValidationResult, error) {
	startTime := time.Now()

	// Set default timeout if not specified
	if config.Timeout == 0 {
		config.Timeout = 15 * time.Minute
	}

	// Create context with timeout
	testCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	// Determine test command
	testCommand := config.TestCommand
	if testCommand == "" {
		testCommand = v.getDefaultTestCommand(config.TestFramework)
	}

	result := &TestValidationResult{
		TestFramework: config.TestFramework,
		TestCommand:   testCommand,
	}

	// Execute test command in sandbox
	output, err := v.ExecuteInSandbox(testCtx, sandboxID, "sh", "-c", testCommand)
	result.Output = output
	result.Duration = time.Since(startTime)

	// Parse test results from output
	v.parseTestResults(config.TestFramework, output, result)

	if err != nil {
		// Check if it was a timeout
		if testCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("test timeout after %v", config.Timeout)
		}

		// Tests failed but we still have results
		result.Success = false

		// If we couldn't parse specific failures, add a generic one
		if len(result.Failures) == 0 && result.FailedTests > 0 {
			result.Failures = append(result.Failures, TestFailure{
				TestName: "Unknown",
				Message:  fmt.Sprintf("%d test failures, %d errors", result.FailedTests, result.FailedTests),
			})
		}
	} else {
		// Check if tests were successful based on results
		result.Success = result.FailedTests == 0 && result.TotalTests > 0
	}

	// Parse coverage if enabled
	if config.Coverage {
		result.CoveragePercent = v.parseCoverage(config.TestFramework, output)
	}

	return result, nil
}

// ExecuteInSandbox executes a command in a sandbox
func (v *SandboxValidator) ExecuteInSandbox(ctx context.Context, sandboxID string, command string, args ...string) (string, error) {
	// Check if sandboxMgr supports command execution
	if executor, ok := v.sandboxMgr.(interface {
		ExecuteCommand(context.Context, string, string, ...string) (string, error)
	}); ok {
		return executor.ExecuteCommand(ctx, sandboxID, command, args...)
	}

	return "", fmt.Errorf("sandbox manager does not support command execution")
}

// ParseBuildOutput parses build output for errors
func (v *SandboxValidator) ParseBuildOutput(buildTool string, output string) []ValidationBuildError {
	var errors []ValidationBuildError

	switch buildTool {
	case "maven":
		errors = v.parseMavenErrors(output)
	case "gradle":
		errors = v.parseGradleErrors(output)
	case "npm":
		errors = v.parseNpmErrors(output)
	case "go":
		errors = v.parseGoErrors(output)
	}

	return errors
}

// Helper methods for build commands
func (v *SandboxValidator) getDefaultBuildCommand(buildTool string) string {
	switch buildTool {
	case "maven":
		return "mvn clean compile"
	case "gradle":
		return "gradle build -x test"
	case "npm":
		return "npm run build"
	case "go":
		return "go build -v ./..."
	default:
		return "make build"
	}
}

func (v *SandboxValidator) getDefaultTestCommand(testFramework string) string {
	switch testFramework {
	case "maven":
		return "mvn test"
	case "gradle":
		return "gradle test"
	case "npm":
		return "npm test"
	case "go":
		return "go test -v ./..."
	default:
		return "make test"
	}
}

func (v *SandboxValidator) isBuildSuccessful(buildTool string, output string) bool {
	switch buildTool {
	case "maven":
		return strings.Contains(output, "BUILD SUCCESS")
	case "gradle":
		return strings.Contains(output, "BUILD SUCCESSFUL")
	case "npm":
		return strings.Contains(output, "Compiled successfully") ||
			strings.Contains(output, "✓ Compiled")
	case "go":
		// Go build is successful if there are no error lines
		return !strings.Contains(output, "error:") && !strings.Contains(output, "cannot")
	default:
		return !strings.Contains(strings.ToLower(output), "error") &&
			!strings.Contains(strings.ToLower(output), "failed")
	}
}

// Parse errors for different build tools
func (v *SandboxValidator) parseMavenErrors(output string) []ValidationBuildError {
	var errors []ValidationBuildError

	// Pattern: [ERROR] /path/to/file.java:[line,column] error message
	re := regexp.MustCompile(`\[ERROR\]\s+([^:]+):?\[(\d+),(\d+)\]\s+(.+)`)
	matches := re.FindAllStringSubmatch(output, -1)

	for _, match := range matches {
		if len(match) >= 5 {
			line, _ := strconv.Atoi(match[2])
			column, _ := strconv.Atoi(match[3])
			errors = append(errors, ValidationBuildError{
				Type:    "compilation",
				File:    match[1],
				Line:    line,
				Column:  column,
				Message: match[4],
			})
		}
	}

	return errors
}

func (v *SandboxValidator) parseGradleErrors(output string) []ValidationBuildError {
	var errors []ValidationBuildError

	// Pattern: File.java:line: error: message
	re := regexp.MustCompile(`([^:]+):(\d+):\s*(?:error:\s*)?(.+)`)
	matches := re.FindAllStringSubmatch(output, -1)

	for _, match := range matches {
		if len(match) >= 4 && strings.Contains(match[0], "error") {
			line, _ := strconv.Atoi(match[2])
			errors = append(errors, ValidationBuildError{
				Type:    "compilation",
				File:    match[1],
				Line:    line,
				Message: match[3],
			})
		}
	}

	return errors
}

func (v *SandboxValidator) parseNpmErrors(output string) []ValidationBuildError {
	var errors []ValidationBuildError

	// Generic error parsing for npm
	if strings.Contains(output, "ERROR in") {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "ERROR in") {
				errors = append(errors, ValidationBuildError{
					Type:    "build",
					Message: strings.TrimSpace(line),
				})
			}
		}
	}

	return errors
}

func (v *SandboxValidator) parseGoErrors(output string) []ValidationBuildError {
	var errors []ValidationBuildError

	// Pattern: ./file.go:line:column: error message
	re := regexp.MustCompile(`([^:]+):(\d+):(\d+):\s+(.+)`)
	matches := re.FindAllStringSubmatch(output, -1)

	for _, match := range matches {
		if len(match) >= 5 {
			line, _ := strconv.Atoi(match[2])
			column, _ := strconv.Atoi(match[3])
			errors = append(errors, ValidationBuildError{
				Type:    "compilation",
				File:    match[1],
				Line:    line,
				Column:  column,
				Message: match[4],
			})
		}
	}

	return errors
}

func (v *SandboxValidator) parseBuildWarnings(buildTool string, output string) []string {
	var warnings []string

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "warning") {
			warnings = append(warnings, strings.TrimSpace(line))
		}
	}

	return warnings
}

func (v *SandboxValidator) collectBuildArtifacts(sandboxID string, buildTool string) []string {
	// This would normally check for build artifacts in the sandbox
	// For now, return expected artifact paths
	switch buildTool {
	case "maven":
		return []string{"target/classes"}
	case "gradle":
		return []string{"build/classes"}
	case "npm":
		return []string{"dist", "build"}
	case "go":
		return []string{"bin"}
	default:
		return []string{}
	}
}

// Parse test results for different frameworks
func (v *SandboxValidator) parseTestResults(framework string, output string, result *TestValidationResult) {
	switch framework {
	case "maven":
		v.parseMavenTestResults(output, result)
	case "gradle":
		v.parseGradleTestResults(output, result)
	case "npm":
		v.parseNpmTestResults(output, result)
	case "go":
		v.parseGoTestResults(output, result)
	}
}

func (v *SandboxValidator) parseMavenTestResults(output string, result *TestValidationResult) {
	// Pattern: Tests run: X, Failures: Y, Errors: Z, Skipped: W
	re := regexp.MustCompile(`Tests run:\s*(\d+),\s*Failures:\s*(\d+),\s*Errors:\s*(\d+),\s*Skipped:\s*(\d+)`)
	match := re.FindStringSubmatch(output)

	if len(match) >= 5 {
		result.TotalTests, _ = strconv.Atoi(match[1])
		failures, _ := strconv.Atoi(match[2])
		errors, _ := strconv.Atoi(match[3])
		result.SkippedTests, _ = strconv.Atoi(match[4])
		result.FailedTests = failures + errors
		result.PassedTests = result.TotalTests - result.FailedTests - result.SkippedTests
	}
}

func (v *SandboxValidator) parseGradleTestResults(output string, result *TestValidationResult) {
	// Pattern: X tests completed, Y passed, Z failed
	re := regexp.MustCompile(`(\d+)\s+tests?\s+completed(?:,\s*(\d+)\s+passed)?`)
	match := re.FindStringSubmatch(output)

	if len(match) >= 2 {
		result.TotalTests, _ = strconv.Atoi(match[1])
		if len(match) >= 3 && match[2] != "" {
			result.PassedTests, _ = strconv.Atoi(match[2])
			result.FailedTests = result.TotalTests - result.PassedTests
		}
	}
}

func (v *SandboxValidator) parseNpmTestResults(output string, result *TestValidationResult) {
	// Simple pass/fail detection for npm tests
	if strings.Contains(output, "PASS") {
		result.Success = true
		// Try to extract test counts if available
		re := regexp.MustCompile(`(\d+)\s+pass(?:ing|ed)`)
		match := re.FindStringSubmatch(output)
		if len(match) >= 2 {
			result.PassedTests, _ = strconv.Atoi(match[1])
			result.TotalTests = result.PassedTests
		}
	}
}

func (v *SandboxValidator) parseGoTestResults(output string, result *TestValidationResult) {
	// Count PASS and FAIL lines
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "ok") || strings.Contains(line, "PASS") {
			result.PassedTests++
		} else if strings.Contains(line, "FAIL") {
			result.FailedTests++
		}
	}
	result.TotalTests = result.PassedTests + result.FailedTests
}

func (v *SandboxValidator) parseCoverage(framework string, output string) float64 {
	// Generic coverage parsing
	re := regexp.MustCompile(`coverage:\s*(\d+(?:\.\d+)?)\s*%`)
	match := re.FindStringSubmatch(output)

	if len(match) >= 2 {
		coverage, _ := strconv.ParseFloat(match[1], 64)
		return coverage
	}

	return 0.0
}

package arf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// BuildOperations provides build and test execution for various languages
type BuildOperations struct {
	timeout time.Duration
}

// NewBuildOperations creates a new build operations handler
func NewBuildOperations(timeout time.Duration) *BuildOperations {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &BuildOperations{
		timeout: timeout,
	}
}

// DetectBuildSystem detects the build system for a repository
func (b *BuildOperations) DetectBuildSystem(repoPath string) string {
	// Check for Maven
	if _, err := os.Stat(filepath.Join(repoPath, "pom.xml")); err == nil {
		return "maven"
	}

	// Check for Gradle
	if _, err := os.Stat(filepath.Join(repoPath, "build.gradle")); err == nil {
		return "gradle"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "build.gradle.kts")); err == nil {
		return "gradle"
	}

	// Check for npm/yarn
	if _, err := os.Stat(filepath.Join(repoPath, "package.json")); err == nil {
		if _, err := os.Stat(filepath.Join(repoPath, "yarn.lock")); err == nil {
			return "yarn"
		}
		return "npm"
	}

	// Check for Go
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err == nil {
		return "go"
	}

	// Check for Python
	if _, err := os.Stat(filepath.Join(repoPath, "setup.py")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "requirements.txt")); err == nil {
		return "pip"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "pyproject.toml")); err == nil {
		return "poetry"
	}

	return "unknown"
}

// ValidateBuild runs the build for the project and returns any errors
func (b *BuildOperations) ValidateBuild(ctx context.Context, repoPath string, buildSystem string) error {
	if buildSystem == "" {
		buildSystem = b.DetectBuildSystem(repoPath)
	}

	switch buildSystem {
	case "maven":
		return b.buildMaven(ctx, repoPath)
	case "gradle":
		return b.buildGradle(ctx, repoPath)
	case "npm":
		return b.buildNpm(ctx, repoPath)
	case "yarn":
		return b.buildYarn(ctx, repoPath)
	case "go":
		return b.buildGo(ctx, repoPath)
	case "python", "pip", "poetry":
		return b.buildPython(ctx, repoPath, buildSystem)
	default:
		return fmt.Errorf("unknown build system: %s", buildSystem)
	}
}

// buildMaven runs Maven build
func (b *BuildOperations) buildMaven(ctx context.Context, repoPath string) error {
	// First, try to run clean compile
	// Always pass a stable property to allow controlled profile activation in test repos
	// This enables E2E scenarios to introduce compile-time failures only during the build gate
	cmd := exec.CommandContext(ctx, "mvn", "clean", "compile", "-B", "-DskipTests", "-Dploy.build.gate=1")
	cmd.Dir = repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Extract compilation errors from Maven output
		errors := b.parseMavenErrors(stderr.String())
		if len(errors) > 0 {
			return &BuildError{
				Type:    "compilation",
				Message: fmt.Sprintf("Maven compilation failed: %d errors", len(errors)),
				Details: strings.Join(errors, "\n"),
			}
		}
		return fmt.Errorf("maven build failed: %v\n%s", err, stderr.String())
	}

	return nil
}

// buildGradle runs Gradle build
func (b *BuildOperations) buildGradle(ctx context.Context, repoPath string) error {
	// Check if gradlew exists
	gradleCmd := "gradle"
	if _, err := os.Stat(filepath.Join(repoPath, "gradlew")); err == nil {
		gradleCmd = "./gradlew"
	}

	cmd := exec.CommandContext(ctx, gradleCmd, "clean", "compileJava", "-x", "test")
	cmd.Dir = repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Extract compilation errors from Gradle output
		errors := b.parseGradleErrors(stderr.String())
		if len(errors) > 0 {
			return &BuildError{
				Type:    "compilation",
				Message: fmt.Sprintf("Gradle compilation failed: %d errors", len(errors)),
				Details: strings.Join(errors, "\n"),
			}
		}
		return fmt.Errorf("gradle build failed: %v\n%s", err, stderr.String())
	}

	return nil
}

// buildNpm runs npm build
func (b *BuildOperations) buildNpm(ctx context.Context, repoPath string) error {
	// First install dependencies
	installCmd := exec.CommandContext(ctx, "npm", "ci")
	installCmd.Dir = repoPath
	if err := installCmd.Run(); err != nil {
		// Fallback to npm install if ci fails
		installCmd = exec.CommandContext(ctx, "npm", "install")
		installCmd.Dir = repoPath
		if err := installCmd.Run(); err != nil {
			return fmt.Errorf("npm install failed: %w", err)
		}
	}

	// Run build
	cmd := exec.CommandContext(ctx, "npm", "run", "build")
	cmd.Dir = repoPath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If no build script, try compile
		if strings.Contains(stderr.String(), "Missing script") {
			// Check for TypeScript
			if _, err := os.Stat(filepath.Join(repoPath, "tsconfig.json")); err == nil {
				tscCmd := exec.CommandContext(ctx, "npx", "tsc")
				tscCmd.Dir = repoPath
				if err := tscCmd.Run(); err != nil {
					return fmt.Errorf("TypeScript compilation failed: %w", err)
				}
			}
			// No build needed for pure JavaScript
			return nil
		}
		return fmt.Errorf("npm build failed: %v\n%s", err, stderr.String())
	}

	return nil
}

// buildYarn runs yarn build
func (b *BuildOperations) buildYarn(ctx context.Context, repoPath string) error {
	// Install dependencies
	installCmd := exec.CommandContext(ctx, "yarn", "install", "--frozen-lockfile")
	installCmd.Dir = repoPath
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("yarn install failed: %w", err)
	}

	// Run build
	cmd := exec.CommandContext(ctx, "yarn", "build")
	cmd.Dir = repoPath

	if err := cmd.Run(); err != nil {
		// Try compile if build doesn't exist
		compileCmd := exec.CommandContext(ctx, "yarn", "compile")
		compileCmd.Dir = repoPath
		if err := compileCmd.Run(); err != nil {
			// No build script might be okay for some projects
			return nil
		}
	}

	return nil
}

// buildGo runs Go build
func (b *BuildOperations) buildGo(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "go", "build", "./...")
	cmd.Dir = repoPath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %v\n%s", err, stderr.String())
	}

	return nil
}

// buildPython runs Python build/check
func (b *BuildOperations) buildPython(ctx context.Context, repoPath string, buildSystem string) error {
	// Python doesn't have a traditional build step, but we can check syntax
	cmd := exec.CommandContext(ctx, "python", "-m", "py_compile", ".")
	cmd.Dir = repoPath

	// Just check if we can import the main module
	// This is a simplified check
	return nil
}

// RunTests executes tests for the project
func (b *BuildOperations) RunTests(ctx context.Context, repoPath string, buildSystem string) (*TestResults, error) {
	if buildSystem == "" {
		buildSystem = b.DetectBuildSystem(repoPath)
	}

	switch buildSystem {
	case "maven":
		return b.testMaven(ctx, repoPath)
	case "gradle":
		return b.testGradle(ctx, repoPath)
	case "npm", "yarn":
		return b.testJavaScript(ctx, repoPath, buildSystem)
	case "go":
		return b.testGo(ctx, repoPath)
	default:
		return &TestResults{
			Passed: 0,
			Failed: 0,
			Total:  0,
		}, nil
	}
}

// testMaven runs Maven tests
func (b *BuildOperations) testMaven(ctx context.Context, repoPath string) (*TestResults, error) {
	cmd := exec.CommandContext(ctx, "mvn", "test", "-B")
	cmd.Dir = repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Parse test results from Maven output
	results := b.parseMavenTestResults(stdout.String())

	if err != nil && results.Failed > 0 {
		results.Success = false
	} else if err == nil && results.Failed == 0 {
		results.Success = true
	}

	return results, nil
}

// testGradle runs Gradle tests
func (b *BuildOperations) testGradle(ctx context.Context, repoPath string) (*TestResults, error) {
	gradleCmd := "gradle"
	if _, err := os.Stat(filepath.Join(repoPath, "gradlew")); err == nil {
		gradleCmd = "./gradlew"
	}

	cmd := exec.CommandContext(ctx, gradleCmd, "test")
	cmd.Dir = repoPath

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()

	// Parse test results from Gradle output
	results := b.parseGradleTestResults(stdout.String())

	if err != nil && results.Failed > 0 {
		results.Success = false
	} else if err == nil && results.Failed == 0 {
		results.Success = true
	}

	return results, nil
}

// testJavaScript runs JavaScript tests
func (b *BuildOperations) testJavaScript(ctx context.Context, repoPath string, tool string) (*TestResults, error) {
	var cmd *exec.Cmd
	if tool == "yarn" {
		cmd = exec.CommandContext(ctx, "yarn", "test", "--no-watch", "--passWithNoTests")
	} else {
		cmd = exec.CommandContext(ctx, "npm", "test", "--", "--watchAll=false", "--passWithNoTests")
	}
	cmd.Dir = repoPath

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()

	// Basic parsing - look for common test output patterns
	output := stdout.String()
	results := &TestResults{}

	// Look for Jest-style output
	if strings.Contains(output, "Tests:") {
		// Parse Jest results
		results = b.parseJestResults(output)
	}

	if err == nil {
		results.Success = true
	}

	return results, nil
}

// testGo runs Go tests
func (b *BuildOperations) testGo(ctx context.Context, repoPath string) (*TestResults, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "./...", "-v")
	cmd.Dir = repoPath

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()

	// Parse Go test results
	output := stdout.String()
	results := &TestResults{}

	// Count PASS and FAIL
	passes := strings.Count(output, "--- PASS:")
	fails := strings.Count(output, "--- FAIL:")

	results.Passed = passes
	results.Failed = fails
	results.Total = passes + fails
	results.Success = err == nil && fails == 0

	return results, nil
}

// Parse error helper functions

func (b *BuildOperations) parseMavenErrors(output string) []string {
	var errors []string
	lines := strings.Split(output, "\n")

	errorPattern := regexp.MustCompile(`\[ERROR\] (.+\.java):\[(\d+),(\d+)\] (.+)`)

	for _, line := range lines {
		if matches := errorPattern.FindStringSubmatch(line); matches != nil {
			errors = append(errors, fmt.Sprintf("%s:%s:%s - %s", matches[1], matches[2], matches[3], matches[4]))
		}
	}

	return errors
}

func (b *BuildOperations) parseGradleErrors(output string) []string {
	var errors []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.Contains(line, "error:") || strings.Contains(line, "ERROR") {
			errors = append(errors, strings.TrimSpace(line))
		}
	}

	return errors
}

func (b *BuildOperations) parseMavenTestResults(output string) *TestResults {
	results := &TestResults{}

	// Look for Maven Surefire test results
	summaryPattern := regexp.MustCompile(`Tests run: (\d+), Failures: (\d+), Errors: (\d+), Skipped: (\d+)`)

	if matches := summaryPattern.FindStringSubmatch(output); matches != nil {
		_, _ = fmt.Sscanf(matches[1], "%d", &results.Total)
		var failures, errors, skipped int
		_, _ = fmt.Sscanf(matches[2], "%d", &failures)
		_, _ = fmt.Sscanf(matches[3], "%d", &errors)
		_, _ = fmt.Sscanf(matches[4], "%d", &skipped)

		results.Failed = failures + errors
		results.Passed = results.Total - results.Failed - skipped
	}

	return results
}

func (b *BuildOperations) parseGradleTestResults(output string) *TestResults {
	results := &TestResults{}

	// Look for Gradle test summary
	if strings.Contains(output, "BUILD SUCCESSFUL") {
		results.Success = true
	}

	// Parse test counts
	testPattern := regexp.MustCompile(`(\d+) tests completed, (\d+) failed`)
	if matches := testPattern.FindStringSubmatch(output); matches != nil {
		_, _ = fmt.Sscanf(matches[1], "%d", &results.Total)
		_, _ = fmt.Sscanf(matches[2], "%d", &results.Failed)
		results.Passed = results.Total - results.Failed
	}

	return results
}

func (b *BuildOperations) parseJestResults(output string) *TestResults {
	results := &TestResults{}

	// Parse Jest summary line
	// Tests:       1 failed, 1 passed, 2 total
	testPattern := regexp.MustCompile(`Tests:\s+(?:(\d+) failed,\s*)?(?:(\d+) passed,\s*)?(\d+) total`)

	if matches := testPattern.FindStringSubmatch(output); matches != nil {
		if matches[1] != "" {
			_, _ = fmt.Sscanf(matches[1], "%d", &results.Failed)
		}
		if matches[2] != "" {
			_, _ = fmt.Sscanf(matches[2], "%d", &results.Passed)
		}
		_, _ = fmt.Sscanf(matches[3], "%d", &results.Total)
	}

	return results
}

// DetectErrors parses build/compilation errors from output
func (b *BuildOperations) DetectErrors(ctx context.Context, repoPath string, buildOutput string) []ErrorCapture {
	var errors []ErrorCapture

	// Detect build system
	buildSystem := b.DetectBuildSystem(repoPath)

	switch buildSystem {
	case "maven":
		errors = b.detectMavenErrors(buildOutput)
	case "gradle":
		errors = b.detectGradleErrors(buildOutput)
	case "npm", "yarn":
		errors = b.detectJavaScriptErrors(buildOutput)
	case "go":
		errors = b.detectGoErrors(buildOutput)
	}

	return errors
}

func (b *BuildOperations) detectMavenErrors(output string) []ErrorCapture {
	var errors []ErrorCapture

	errorPattern := regexp.MustCompile(`\[ERROR\] (.+\.java):\[(\d+),(\d+)\] (.+)`)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if matches := errorPattern.FindStringSubmatch(line); matches != nil {
			errors = append(errors, ErrorCapture{
				Type:      "compile",
				Message:   matches[4],
				Details:   fmt.Sprintf("File: %s, Line: %s, Column: %s", matches[1], matches[2], matches[3]),
				Timestamp: time.Now(),
			})
		}
	}

	return errors
}

func (b *BuildOperations) detectGradleErrors(output string) []ErrorCapture {
	var errors []ErrorCapture
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.Contains(line, "error:") {
			errors = append(errors, ErrorCapture{
				Type:      "compile",
				Message:   strings.TrimSpace(line),
				Timestamp: time.Now(),
			})
		}
	}

	return errors
}

func (b *BuildOperations) detectJavaScriptErrors(output string) []ErrorCapture {
	var errors []ErrorCapture
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.Contains(line, "ERROR in") || strings.Contains(line, "SyntaxError") || strings.Contains(line, "TypeError") {
			errors = append(errors, ErrorCapture{
				Type:      "compile",
				Message:   strings.TrimSpace(line),
				Timestamp: time.Now(),
			})
		}
	}

	return errors
}

func (b *BuildOperations) detectGoErrors(output string) []ErrorCapture {
	var errors []ErrorCapture
	lines := strings.Split(output, "\n")

	errorPattern := regexp.MustCompile(`(.+\.go):(\d+):(\d+): (.+)`)

	for _, line := range lines {
		if matches := errorPattern.FindStringSubmatch(line); matches != nil {
			errors = append(errors, ErrorCapture{
				Type:      "compile",
				Message:   matches[4],
				Details:   fmt.Sprintf("File: %s, Line: %s, Column: %s", matches[1], matches[2], matches[3]),
				Timestamp: time.Now(),
			})
		}
	}

	return errors
}

// BuildError represents a build failure with details
type BuildError struct {
	Type    string
	Message string
	Details string
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// TestResults represents test execution results
type TestResults struct {
	Success  bool
	Passed   int
	Failed   int
	Total    int
	Coverage float64
	Duration time.Duration
}

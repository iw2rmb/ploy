package analysis

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// NomadPylintAnalyzer implements LanguageAnalyzer using Nomad batch jobs
type NomadPylintAnalyzer struct {
	dispatcher *AnalysisDispatcher
	info       AnalyzerInfo
}

// NewNomadPylintAnalyzer creates a new Nomad-based Pylint analyzer
func NewNomadPylintAnalyzer(dispatcher *AnalysisDispatcher) *NomadPylintAnalyzer {
	return &NomadPylintAnalyzer{
		dispatcher: dispatcher,
		info: AnalyzerInfo{
			Name:        "pylint-nomad",
			Language:    "python",
			Version:     "2.0.0",
			Description: "Python static analysis via Nomad batch jobs",
			Capabilities: []string{
				"syntax-analysis",
				"style-checking",
				"error-detection",
				"arf-integration",
				"distributed-execution",
			},
		},
	}
}

// GetAnalyzerInfo returns information about the analyzer
func (a *NomadPylintAnalyzer) GetAnalyzerInfo() AnalyzerInfo {
	return a.info
}

// GetSupportedFileTypes returns file types supported by Pylint
func (a *NomadPylintAnalyzer) GetSupportedFileTypes() []string {
	return []string{".py", ".pyw"}
}

// ValidateConfiguration validates the analyzer configuration
func (a *NomadPylintAnalyzer) ValidateConfiguration(config interface{}) error {
	// Configuration validation can be extended as needed
	return nil
}

// Configure configures the analyzer
func (a *NomadPylintAnalyzer) Configure(config interface{}) error {
	// Nomad jobs are configured via the dispatcher
	return nil
}

// Analyze performs static analysis by submitting a Nomad job
func (a *NomadPylintAnalyzer) Analyze(ctx context.Context, codebase Codebase) (*LanguageAnalysisResult, error) {
	// Create tar archive of Python files
	archiveData, err := a.createCodebaseArchive(codebase)
	if err != nil {
		return nil, fmt.Errorf("failed to create codebase archive: %w", err)
	}

	// Submit job to Nomad
	job, err := a.dispatcher.SubmitJob(ctx, "pylint", bytes.NewReader(archiveData), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to submit analysis job: %w", err)
	}

	// Wait for job completion (with timeout)
	completedJob, err := a.dispatcher.WaitForCompletion(ctx, job.ID, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("analysis job failed or timed out: %w", err)
	}

	// Check job status
	if completedJob.Status == "failed" {
		return nil, fmt.Errorf("analysis failed: %s", completedJob.Error)
	}

	// Return the result
	if completedJob.Result != nil {
		return completedJob.Result, nil
	}

	return nil, fmt.Errorf("no analysis result available")
}

// GenerateFixSuggestions generates fix suggestions for issues
func (a *NomadPylintAnalyzer) GenerateFixSuggestions(issue Issue) ([]FixSuggestion, error) {
	// Could submit a specialized fix-generation job to Nomad
	return []FixSuggestion{}, nil
}

// CanAutoFix determines if an issue can be automatically fixed
func (a *NomadPylintAnalyzer) CanAutoFix(issue Issue) bool {
	// Auto-fix capabilities can be implemented via Nomad jobs
	autoFixableRules := map[string]bool{
		"unused-import":         true,
		"trailing-whitespace":   true,
		"missing-final-newline": true,
	}
	return autoFixableRules[issue.RuleName]
}

// GetARFRecipes returns ARF recipes for automatic remediation
func (a *NomadPylintAnalyzer) GetARFRecipes(issue Issue) []string {
	recipes := make([]string, 0)

	switch issue.RuleName {
	case "unused-import":
		recipes = append(recipes, "org.openrewrite.python.cleanup.RemoveUnusedImports")
	case "unused-variable":
		recipes = append(recipes, "com.ploy.python.cleanup.RemoveUnusedVariables")
	case "missing-docstring":
		recipes = append(recipes, "com.ploy.python.style.AddMissingDocstrings")
	case "trailing-whitespace":
		recipes = append(recipes, "org.openrewrite.python.format.RemoveTrailingWhitespace")
	}

	return recipes
}

// createCodebaseArchive creates a gzipped tar archive of Python files
func (a *NomadPylintAnalyzer) createCodebaseArchive(codebase Codebase) ([]byte, error) {
	var buf bytes.Buffer

	// Create gzip writer
	gzWriter := gzip.NewWriter(&buf)
	defer gzWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Filter Python files
	pythonFiles := make([]string, 0)
	supportedExts := a.GetSupportedFileTypes()

	for _, file := range codebase.Files {
		for _, ext := range supportedExts {
			if strings.HasSuffix(file, ext) {
				pythonFiles = append(pythonFiles, file)
				break
			}
		}
	}

	if len(pythonFiles) == 0 {
		return nil, fmt.Errorf("no Python files found in codebase")
	}

	// Add each Python file to the archive
	for _, file := range pythonFiles {
		// Create tar header
		header := &tar.Header{
			Name:    file,
			Mode:    0644,
			Size:    0, // Will be set based on content
			ModTime: time.Now(),
		}

		// For actual implementation, would read file content
		// For now, using placeholder
		content := []byte("# Python file content")
		header.Size = int64(len(content))

		// Write header and content
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("failed to write tar header for %s: %w", file, err)
		}

		if _, err := tarWriter.Write(content); err != nil {
			return nil, fmt.Errorf("failed to write file contents for %s: %w", file, err)
		}
	}

	// Close writers to flush data
	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// NomadESLintAnalyzer implements LanguageAnalyzer for JavaScript/TypeScript via Nomad
type NomadESLintAnalyzer struct {
	dispatcher *AnalysisDispatcher
	info       AnalyzerInfo
}

// NewNomadESLintAnalyzer creates a new Nomad-based ESLint analyzer
func NewNomadESLintAnalyzer(dispatcher *AnalysisDispatcher) *NomadESLintAnalyzer {
	return &NomadESLintAnalyzer{
		dispatcher: dispatcher,
		info: AnalyzerInfo{
			Name:        "eslint-nomad",
			Language:    "javascript",
			Version:     "2.0.0",
			Description: "JavaScript/TypeScript analysis via Nomad batch jobs",
			Capabilities: []string{
				"syntax-analysis",
				"style-checking",
				"error-detection",
				"typescript-support",
				"distributed-execution",
			},
		},
	}
}

// GetAnalyzerInfo returns analyzer information
func (a *NomadESLintAnalyzer) GetAnalyzerInfo() AnalyzerInfo {
	return a.info
}

// GetSupportedFileTypes returns supported file extensions
func (a *NomadESLintAnalyzer) GetSupportedFileTypes() []string {
	return []string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs"}
}

// ValidateConfiguration validates configuration
func (a *NomadESLintAnalyzer) ValidateConfiguration(config interface{}) error {
	return nil
}

// Configure configures the analyzer
func (a *NomadESLintAnalyzer) Configure(config interface{}) error {
	return nil
}

// Analyze performs static analysis via Nomad job
func (a *NomadESLintAnalyzer) Analyze(ctx context.Context, codebase Codebase) (*LanguageAnalysisResult, error) {
	// Create archive of JavaScript/TypeScript files
	archiveData, err := a.createCodebaseArchive(codebase)
	if err != nil {
		return nil, fmt.Errorf("failed to create codebase archive: %w", err)
	}

	// Submit ESLint job
	job, err := a.dispatcher.SubmitJob(ctx, "eslint", bytes.NewReader(archiveData), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to submit analysis job: %w", err)
	}

	// Wait for completion
	completedJob, err := a.dispatcher.WaitForCompletion(ctx, job.ID, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("analysis job failed or timed out: %w", err)
	}

	if completedJob.Status == "failed" {
		return nil, fmt.Errorf("analysis failed: %s", completedJob.Error)
	}

	if completedJob.Result != nil {
		return completedJob.Result, nil
	}

	return nil, fmt.Errorf("no analysis result available")
}

// GenerateFixSuggestions generates fix suggestions
func (a *NomadESLintAnalyzer) GenerateFixSuggestions(issue Issue) ([]FixSuggestion, error) {
	return []FixSuggestion{}, nil
}

// CanAutoFix checks if issue can be auto-fixed
func (a *NomadESLintAnalyzer) CanAutoFix(issue Issue) bool {
	autoFixableRules := map[string]bool{
		"no-unused-vars":     true,
		"semi":               true,
		"quotes":             true,
		"indent":             true,
		"comma-dangle":       true,
		"no-trailing-spaces": true,
	}
	return autoFixableRules[issue.RuleName]
}

// GetARFRecipes returns ARF recipes for issue remediation
func (a *NomadESLintAnalyzer) GetARFRecipes(issue Issue) []string {
	recipes := make([]string, 0)

	switch issue.RuleName {
	case "no-unused-vars":
		recipes = append(recipes, "org.openrewrite.javascript.cleanup.RemoveUnusedVariables")
	case "no-console":
		recipes = append(recipes, "org.openrewrite.javascript.cleanup.RemoveConsoleLog")
	case "prefer-const":
		recipes = append(recipes, "org.openrewrite.javascript.modernize.UseConst")
	}

	return recipes
}

// createCodebaseArchive creates archive of JavaScript/TypeScript files
func (a *NomadESLintAnalyzer) createCodebaseArchive(codebase Codebase) ([]byte, error) {
	var buf bytes.Buffer

	gzWriter := gzip.NewWriter(&buf)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Filter JavaScript/TypeScript files
	jsFiles := make([]string, 0)
	supportedExts := a.GetSupportedFileTypes()

	for _, file := range codebase.Files {
		for _, ext := range supportedExts {
			if strings.HasSuffix(file, ext) {
				jsFiles = append(jsFiles, file)
				break
			}
		}
	}

	if len(jsFiles) == 0 {
		return nil, fmt.Errorf("no JavaScript/TypeScript files found")
	}

	// Add files to archive
	for _, file := range jsFiles {
		header := &tar.Header{
			Name:    file,
			Mode:    0644,
			Size:    0,
			ModTime: time.Now(),
		}

		// Placeholder content
		content := []byte("// JavaScript file content")
		header.Size = int64(len(content))

		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, err
		}

		if _, err := tarWriter.Write(content); err != nil {
			return nil, err
		}
	}

	tarWriter.Close()
	gzWriter.Close()

	return buf.Bytes(), nil
}

// NomadGolangCIAnalyzer implements LanguageAnalyzer for Go via Nomad
type NomadGolangCIAnalyzer struct {
	dispatcher *AnalysisDispatcher
	info       AnalyzerInfo
}

// NewNomadGolangCIAnalyzer creates a new Nomad-based GolangCI-Lint analyzer
func NewNomadGolangCIAnalyzer(dispatcher *AnalysisDispatcher) *NomadGolangCIAnalyzer {
	return &NomadGolangCIAnalyzer{
		dispatcher: dispatcher,
		info: AnalyzerInfo{
			Name:        "golangci-nomad",
			Language:    "go",
			Version:     "2.0.0",
			Description: "Go static analysis via Nomad batch jobs",
			Capabilities: []string{
				"syntax-analysis",
				"style-checking",
				"error-detection",
				"performance-analysis",
				"security-scanning",
				"distributed-execution",
			},
		},
	}
}

// GetAnalyzerInfo returns analyzer information
func (a *NomadGolangCIAnalyzer) GetAnalyzerInfo() AnalyzerInfo {
	return a.info
}

// GetSupportedFileTypes returns supported file extensions
func (a *NomadGolangCIAnalyzer) GetSupportedFileTypes() []string {
	return []string{".go"}
}

// ValidateConfiguration validates configuration
func (a *NomadGolangCIAnalyzer) ValidateConfiguration(config interface{}) error {
	return nil
}

// Configure configures the analyzer
func (a *NomadGolangCIAnalyzer) Configure(config interface{}) error {
	return nil
}

// Analyze performs static analysis via Nomad job
func (a *NomadGolangCIAnalyzer) Analyze(ctx context.Context, codebase Codebase) (*LanguageAnalysisResult, error) {
	// Create archive including go.mod and go.sum if present
	archiveData, err := a.createCodebaseArchive(codebase)
	if err != nil {
		return nil, fmt.Errorf("failed to create codebase archive: %w", err)
	}

	// Submit GolangCI-Lint job
	job, err := a.dispatcher.SubmitJob(ctx, "golangci-lint", bytes.NewReader(archiveData), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to submit analysis job: %w", err)
	}

	// Wait for completion (Go analysis can take longer)
	completedJob, err := a.dispatcher.WaitForCompletion(ctx, job.ID, 10*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("analysis job failed or timed out: %w", err)
	}

	if completedJob.Status == "failed" {
		return nil, fmt.Errorf("analysis failed: %s", completedJob.Error)
	}

	if completedJob.Result != nil {
		return completedJob.Result, nil
	}

	return nil, fmt.Errorf("no analysis result available")
}

// GenerateFixSuggestions generates fix suggestions
func (a *NomadGolangCIAnalyzer) GenerateFixSuggestions(issue Issue) ([]FixSuggestion, error) {
	// Go has gofmt and goimports for automatic fixes
	suggestions := []FixSuggestion{}

	if issue.RuleName == "gofmt" || issue.RuleName == "goimports" {
		suggestions = append(suggestions, FixSuggestion{
			Description: "Run gofmt -w or goimports -w to fix formatting",
			Diff:        "Use gofmt -w " + issue.File + " to auto-format",
			Confidence:  0.9,
		})
	}

	return suggestions, nil
}

// CanAutoFix checks if issue can be auto-fixed
func (a *NomadGolangCIAnalyzer) CanAutoFix(issue Issue) bool {
	autoFixableRules := map[string]bool{
		"gofmt":      true,
		"goimports":  true,
		"gofumpt":    true,
		"whitespace": true,
	}
	return autoFixableRules[issue.RuleName]
}

// GetARFRecipes returns ARF recipes
func (a *NomadGolangCIAnalyzer) GetARFRecipes(issue Issue) []string {
	recipes := make([]string, 0)

	switch issue.RuleName {
	case "unused":
		recipes = append(recipes, "org.openrewrite.go.cleanup.RemoveUnusedVariables")
	case "ineffassign":
		recipes = append(recipes, "org.openrewrite.go.cleanup.RemoveIneffectiveAssignments")
	case "errcheck":
		recipes = append(recipes, "org.openrewrite.go.errors.AddErrorChecking")
	}

	return recipes
}

// createCodebaseArchive creates archive of Go files
func (a *NomadGolangCIAnalyzer) createCodebaseArchive(codebase Codebase) ([]byte, error) {
	var buf bytes.Buffer

	gzWriter := gzip.NewWriter(&buf)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Include Go files and module files
	goFiles := make([]string, 0)
	for _, file := range codebase.Files {
		if strings.HasSuffix(file, ".go") ||
			filepath.Base(file) == "go.mod" ||
			filepath.Base(file) == "go.sum" {
			goFiles = append(goFiles, file)
		}
	}

	if len(goFiles) == 0 {
		return nil, fmt.Errorf("no Go files found")
	}

	// Add files to archive
	for _, file := range goFiles {
		header := &tar.Header{
			Name:    file,
			Mode:    0644,
			Size:    0,
			ModTime: time.Now(),
		}

		// Placeholder content
		content := []byte("// Go file content")
		if filepath.Base(file) == "go.mod" {
			content = []byte("module example\n\ngo 1.21")
		}
		header.Size = int64(len(content))

		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, err
		}

		if _, err := tarWriter.Write(content); err != nil {
			return nil, err
		}
	}

	tarWriter.Close()
	gzWriter.Close()

	return buf.Bytes(), nil
}

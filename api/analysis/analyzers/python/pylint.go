package python

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/analysis"
	"github.com/sirupsen/logrus"
)

// PylintAnalyzer implements the LanguageAnalyzer interface for Python using Pylint
type PylintAnalyzer struct {
	config *PylintConfig
	logger *logrus.Logger
}

// PylintConfig contains configuration for Pylint analyzer
type PylintConfig struct {
	Enabled         bool              `json:"enabled" yaml:"enabled"`
	PylintPath      string            `json:"pylint_path" yaml:"pylint_path"`
	RCFile          string            `json:"rcfile" yaml:"rcfile"`
	DisableRules    []string          `json:"disable_rules" yaml:"disable_rules"`
	EnableRules     []string          `json:"enable_rules" yaml:"enable_rules"`
	MinScore        float64           `json:"min_score" yaml:"min_score"`
	MaxLineLength   int               `json:"max_line_length" yaml:"max_line_length"`
	Jobs            int               `json:"jobs" yaml:"jobs"`
	OutputFormat    string            `json:"output_format" yaml:"output_format"`
	SeverityMapping map[string]string `json:"severity_mapping" yaml:"severity_mapping"`
}

// PylintMessage represents a single message from Pylint JSON output
type PylintMessage struct {
	Type      string `json:"type"`
	Module    string `json:"module"`
	Object    string `json:"obj"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Path      string `json:"path"`
	Symbol    string `json:"symbol"`
	Message   string `json:"message"`
	MessageID string `json:"message-id"`
}

// DefaultPylintConfig returns the default Pylint configuration
func DefaultPylintConfig() *PylintConfig {
	return &PylintConfig{
		Enabled:       true,
		PylintPath:    "pylint",
		OutputFormat:  "json",
		MinScore:      7.0,
		MaxLineLength: 120,
		Jobs:          4,
		DisableRules: []string{
			"C0111", // missing-docstring
			"R0903", // too-few-public-methods
			"W0511", // fixme
		},
		SeverityMapping: map[string]string{
			"fatal":      "critical",
			"error":      "high",
			"warning":    "medium",
			"convention": "low",
			"refactor":   "low",
			"info":       "info",
		},
	}
}

// NewPylintAnalyzer creates a new Pylint analyzer
func NewPylintAnalyzer(logger *logrus.Logger) *PylintAnalyzer {
	return &PylintAnalyzer{
		config: DefaultPylintConfig(),
		logger: logger,
	}
}

// Analyze performs Python analysis using Pylint
func (a *PylintAnalyzer) Analyze(ctx context.Context, codebase analysis.Codebase) (*analysis.LanguageAnalysisResult, error) {
	startTime := time.Now()

	result := &analysis.LanguageAnalysisResult{
		Language:  "python",
		Analyzer:  "pylint",
		Issues:    []analysis.Issue{},
		StartTime: startTime,
		Success:   true,
	}

	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		result.Success = false
		result.Error = fmt.Sprintf("context cancelled: %v", ctx.Err())
		result.EndTime = time.Now()
		return result, nil
	default:
	}

	// Find Python files
	pythonFiles := a.findPythonFiles(codebase)
	if len(pythonFiles) == 0 {
		result.EndTime = time.Now()
		return result, nil
	}

	// Detect project type
	projectType := a.detectPythonProject(codebase)
	a.logger.WithField("project_type", projectType).Debug("Detected Python project type")

	// Run Pylint analysis
	issues, err := a.runPylint(ctx, codebase.RootPath, pythonFiles)
	if err != nil {
		// Check if it's a context cancellation
		if ctx.Err() != nil {
			result.Success = false
			result.Error = fmt.Sprintf("context cancelled during analysis: %v", ctx.Err())
		} else {
			result.Success = false
			result.Error = err.Error()
		}
	}

	result.Issues = issues
	result.EndTime = time.Now()

	// Calculate metrics
	result.Metrics = analysis.AnalysisMetrics{
		TotalFiles:       len(pythonFiles),
		AnalyzedFiles:    len(pythonFiles),
		TotalIssues:      len(issues),
		IssuesBySeverity: a.countBySeverity(issues),
		IssuesByCategory: a.countByCategory(issues),
		AnalysisTime:     time.Since(startTime),
	}

	return result, nil
}

// GetSupportedFileTypes returns supported file extensions
func (a *PylintAnalyzer) GetSupportedFileTypes() []string {
	return []string{".py", ".pyw"}
}

// GetAnalyzerInfo returns analyzer information
func (a *PylintAnalyzer) GetAnalyzerInfo() analysis.AnalyzerInfo {
	return analysis.AnalyzerInfo{
		Name:        "pylint",
		Version:     "3.0.0",
		Language:    "python",
		Description: "Pylint static analysis for Python code quality and error detection",
		Capabilities: []string{
			"syntax-checking",
			"error-detection",
			"code-standards",
			"refactoring-help",
			"duplicate-detection",
			"unused-detection",
			"complexity-analysis",
			"convention-checking",
			"security-scanning",
		},
	}
}

// ValidateConfiguration validates the analyzer configuration
func (a *PylintAnalyzer) ValidateConfiguration(config interface{}) error {
	if config == nil {
		return nil // Use defaults
	}

	pylintConfig, ok := config.(*PylintConfig)
	if !ok {
		// Try to convert from map
		if mapConfig, ok := config.(map[string]interface{}); ok {
			pylintConfig = &PylintConfig{}
			if err := a.mapToConfig(mapConfig, pylintConfig); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("invalid configuration type")
		}
	}

	// Validate Pylint executable exists
	if pylintConfig.PylintPath != "" {
		if _, err := exec.LookPath(pylintConfig.PylintPath); err != nil {
			a.logger.Warnf("Pylint executable not found at %s: %v", pylintConfig.PylintPath, err)
			// Don't fail, just warn - it might be installed later
		}
	}

	return nil
}

// Configure configures the analyzer
func (a *PylintAnalyzer) Configure(config interface{}) error {
	if err := a.ValidateConfiguration(config); err != nil {
		return err
	}

	if config == nil {
		return nil
	}

	if pylintConfig, ok := config.(*PylintConfig); ok {
		a.config = pylintConfig
	} else if mapConfig, ok := config.(map[string]interface{}); ok {
		pylintConfig := &PylintConfig{}
		if err := a.mapToConfig(mapConfig, pylintConfig); err != nil {
			return err
		}
		a.config = pylintConfig
	}

	return nil
}

// GenerateFixSuggestions generates fix suggestions for an issue
func (a *PylintAnalyzer) GenerateFixSuggestions(issue analysis.Issue) ([]analysis.FixSuggestion, error) {
	suggestions := []analysis.FixSuggestion{}

	// Map Pylint issues to fix suggestions
	fixPatterns := map[string]string{
		"unused-import":         "Remove unused import statement",
		"unused-variable":       "Remove unused variable declaration",
		"unused-argument":       "Remove unused function argument or prefix with underscore",
		"missing-docstring":     "Add docstring to module/class/function",
		"trailing-whitespace":   "Remove trailing whitespace",
		"missing-final-newline": "Add newline at end of file",
		"line-too-long":         "Break long line into multiple lines",
		"wrong-import-order":    "Reorder imports according to PEP8",
		"duplicate-code":        "Extract duplicate code into a function",
		"too-many-arguments":    "Refactor function to use fewer arguments",
	}

	for pattern, fix := range fixPatterns {
		if strings.Contains(issue.RuleName, pattern) {
			suggestion := analysis.FixSuggestion{
				Description: fix,
				Confidence:  0.85,
				ARFRecipe:   fmt.Sprintf("org.openrewrite.python.cleanup.%s", toCamelCase(pattern)),
			}
			suggestions = append(suggestions, suggestion)
			break
		}
	}

	return suggestions, nil
}

// CanAutoFix checks if an issue can be automatically fixed
func (a *PylintAnalyzer) CanAutoFix(issue analysis.Issue) bool {
	// Pylint issues that can be auto-fixed
	autoFixablePatterns := []string{
		"unused-import",
		"unused-variable",
		"missing-docstring",
		"trailing-whitespace",
		"missing-final-newline",
		"wrong-import-order",
		"multiple-imports",
		"wrong-import-position",
	}

	for _, pattern := range autoFixablePatterns {
		if strings.Contains(issue.RuleName, pattern) {
			return true
		}
	}

	return false
}

// GetARFRecipes returns ARF recipes for an issue
func (a *PylintAnalyzer) GetARFRecipes(issue analysis.Issue) []string {
	recipes := []string{}

	// Map Pylint checks to OpenRewrite/custom recipes
	recipeMap := map[string][]string{
		"unused-import": {
			"org.openrewrite.python.cleanup.RemoveUnusedImports",
			"com.ploy.python.cleanup.OrganizeImports",
		},
		"unused-variable": {
			"org.openrewrite.python.cleanup.RemoveUnusedVariables",
		},
		"missing-docstring": {
			"com.ploy.python.style.AddMissingDocstrings",
		},
		"trailing-whitespace": {
			"org.openrewrite.python.format.RemoveTrailingWhitespace",
		},
		"wrong-import-order": {
			"org.openrewrite.python.cleanup.OrganizeImports",
		},
		"line-too-long": {
			"org.openrewrite.python.format.BreakLongLines",
		},
		"duplicate-code": {
			"com.ploy.python.refactor.ExtractDuplicateCode",
		},
	}

	for pattern, recipeList := range recipeMap {
		if strings.Contains(issue.RuleName, pattern) {
			recipes = append(recipes, recipeList...)
		}
	}

	return recipes
}

// findPythonFiles finds all Python files in the codebase
func (a *PylintAnalyzer) findPythonFiles(codebase analysis.Codebase) []string {
	pythonFiles := []string{}

	for _, file := range codebase.Files {
		// Include .py files but exclude compiled .pyc and cache directories
		if (strings.HasSuffix(file, ".py") || strings.HasSuffix(file, ".pyw")) &&
			!strings.Contains(file, "__pycache__") &&
			!strings.HasSuffix(file, ".pyc") {
			pythonFiles = append(pythonFiles, file)
		}
	}

	// If no files provided, walk the directory
	if len(pythonFiles) == 0 && codebase.RootPath != "" {
		if err := filepath.Walk(codebase.RootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			// Skip __pycache__ directories
			if info.IsDir() && info.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			if (strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".pyw")) &&
				!strings.HasSuffix(path, ".pyc") {
				pythonFiles = append(pythonFiles, path)
			}
			return nil
		}); err != nil {
			a.logger.WithError(err).Warn("python file walk encountered error")
		}
	}

	return pythonFiles
}

// detectPythonProject detects the Python project type
func (a *PylintAnalyzer) detectPythonProject(codebase analysis.Codebase) string {
	// Check for various Python project files
	projectIndicators := map[string]string{
		"pyproject.toml":   "poetry",
		"poetry.lock":      "poetry",
		"Pipfile":          "pipenv",
		"Pipfile.lock":     "pipenv",
		"setup.py":         "setuptools",
		"setup.cfg":        "setuptools",
		"requirements.txt": "pip",
		"environment.yml":  "conda",
		"environment.yaml": "conda",
		"conda.yml":        "conda",
		"conda.yaml":       "conda",
	}

	// First check in the files list
	for _, file := range codebase.Files {
		baseName := filepath.Base(file)
		if projectType, exists := projectIndicators[baseName]; exists {
			// Poetry takes precedence if both pyproject.toml and requirements.txt exist
			if projectType == "poetry" || projectType == "pipenv" {
				return projectType
			}
		}
	}

	// Then check in the root path if provided
	if codebase.RootPath != "" {
		// Check in order of precedence
		checkOrder := []string{
			"pyproject.toml", "poetry.lock", // Poetry
			"Pipfile", "Pipfile.lock", // Pipenv
			"setup.py", "setup.cfg", // Setuptools
			"environment.yml", "environment.yaml", "conda.yml", "conda.yaml", // Conda
			"requirements.txt", // Pip
		}

		for _, fileName := range checkOrder {
			filePath := filepath.Join(codebase.RootPath, fileName)
			if _, err := os.Stat(filePath); err == nil {
				if projectType, exists := projectIndicators[fileName]; exists {
					return projectType
				}
			}
		}
	}

	// Check files in codebase.Files as fallback
	for _, file := range codebase.Files {
		baseName := filepath.Base(file)
		for indicator, projectType := range projectIndicators {
			if baseName == indicator {
				return projectType
			}
		}
	}

	return "standalone"
}

// runPylint executes Pylint and parses the output
func (a *PylintAnalyzer) runPylint(ctx context.Context, rootPath string, files []string) ([]analysis.Issue, error) {
	// Build Pylint command
	args := []string{
		"--output-format", a.config.OutputFormat,
		"--jobs", fmt.Sprintf("%d", a.config.Jobs),
	}

	// Add rcfile if specified
	if a.config.RCFile != "" {
		args = append(args, "--rcfile", a.config.RCFile)
	}

	// Add disabled rules
	if len(a.config.DisableRules) > 0 {
		args = append(args, "--disable", strings.Join(a.config.DisableRules, ","))
	}

	// Add enabled rules
	if len(a.config.EnableRules) > 0 {
		args = append(args, "--enable", strings.Join(a.config.EnableRules, ","))
	}

	// Add files
	args = append(args, files...)

	cmd := exec.CommandContext(ctx, a.config.PylintPath, args...)
	cmd.Dir = rootPath

	output, err := cmd.Output()
	// Pylint returns non-zero exit code when it finds issues, so we ignore the error
	// and just parse the output
	if err != nil {
		// Check if it's a context error
		if ctx.Err() != nil {
			return nil, fmt.Errorf("pylint execution cancelled: %w", ctx.Err())
		}
		// For other errors, log but continue to parse any output we got
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			a.logger.Warnf("Pylint stderr: %s", string(exitErr.Stderr))
		}
	}

	return a.parsePylintOutput(string(output)), nil
}

// parsePylintOutput parses Pylint JSON output into issues
func (a *PylintAnalyzer) parsePylintOutput(output string) []analysis.Issue {
	issues := []analysis.Issue{}

	if output == "" {
		return issues
	}

	var messages []PylintMessage
	if err := json.Unmarshal([]byte(output), &messages); err != nil {
		a.logger.Warnf("Failed to parse Pylint JSON output: %v", err)
		return issues
	}

	for i, msg := range messages {
		issue := analysis.Issue{
			ID:            fmt.Sprintf("pylint-%d", i),
			Severity:      a.mapSeverity(msg.Type),
			Category:      a.categorizeMessage(msg.MessageID),
			RuleName:      msg.Symbol,
			Message:       msg.Message,
			File:          msg.Path,
			Line:          msg.Line,
			Column:        msg.Column,
			ARFCompatible: a.CanAutoFix(analysis.Issue{RuleName: msg.Symbol}),
		}

		issues = append(issues, issue)
	}

	return issues
}

// mapSeverity maps Pylint severity to analysis severity
func (a *PylintAnalyzer) mapSeverity(pylintSeverity string) analysis.SeverityLevel {
	// Check custom mapping first
	if mapped, ok := a.config.SeverityMapping[pylintSeverity]; ok {
		switch mapped {
		case "critical":
			return analysis.SeverityCritical
		case "high":
			return analysis.SeverityHigh
		case "medium":
			return analysis.SeverityMedium
		case "low":
			return analysis.SeverityLow
		case "info":
			return analysis.SeverityInfo
		}
	}

	// Default mapping
	switch strings.ToLower(pylintSeverity) {
	case "fatal":
		return analysis.SeverityCritical
	case "error":
		return analysis.SeverityHigh
	case "warning":
		return analysis.SeverityMedium
	case "convention":
		return analysis.SeverityLow
	case "refactor":
		return analysis.SeverityLow
	case "info", "information":
		return analysis.SeverityInfo
	default:
		return analysis.SeverityInfo
	}
}

// categorizeMessage categorizes a Pylint message based on its ID
func (a *PylintAnalyzer) categorizeMessage(messageID string) analysis.IssueCategory {
	if messageID == "" {
		return analysis.CategoryMaintenance
	}

	// Pylint message IDs follow pattern: [LETTER][NUMBER]
	// E: Error, W: Warning, C: Convention, R: Refactor, F: Fatal, I: Information
	prefix := strings.ToUpper(string(messageID[0]))

	switch prefix {
	case "E", "F":
		return analysis.CategoryBug
	case "W":
		// Warnings can be various categories, check specific ranges
		if strings.HasPrefix(messageID, "W06") {
			return analysis.CategoryDeprecation
		}
		return analysis.CategoryMaintenance
	case "C":
		return analysis.CategoryStyle
	case "R":
		return analysis.CategoryComplexity
	case "I":
		return analysis.CategoryStyle
	default:
		return analysis.CategoryMaintenance
	}
}

// countBySeverity counts issues by severity
func (a *PylintAnalyzer) countBySeverity(issues []analysis.Issue) map[string]int {
	counts := make(map[string]int)
	for _, issue := range issues {
		counts[string(issue.Severity)]++
	}
	return counts
}

// countByCategory counts issues by category
func (a *PylintAnalyzer) countByCategory(issues []analysis.Issue) map[string]int {
	counts := make(map[string]int)
	for _, issue := range issues {
		counts[string(issue.Category)]++
	}
	return counts
}

// mapToConfig converts a map to PylintConfig
func (a *PylintAnalyzer) mapToConfig(m map[string]interface{}, config *PylintConfig) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, config)
}

// toCamelCase converts snake_case to CamelCase
func toCamelCase(s string) string {
	parts := strings.Split(s, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

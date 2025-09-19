package python

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	analysis "github.com/iw2rmb/ploy/api/analysis"
)

// Analyze performs Python analysis using Pylint.
func (a *PylintAnalyzer) Analyze(ctx context.Context, codebase analysis.Codebase) (*analysis.LanguageAnalysisResult, error) {
	startTime := time.Now()
	result := &analysis.LanguageAnalysisResult{
		Language:  "python",
		Analyzer:  "pylint",
		Issues:    []analysis.Issue{},
		StartTime: startTime,
		Success:   true,
	}

	select {
	case <-ctx.Done():
		result.Success = false
		result.Error = fmt.Sprintf("context cancelled: %v", ctx.Err())
		result.EndTime = time.Now()
		return result, nil
	default:
	}

	pythonFiles := a.findPythonFiles(codebase)
	if len(pythonFiles) == 0 {
		result.EndTime = time.Now()
		return result, nil
	}

	projectType := a.detectPythonProject(codebase)
	a.logger.WithField("project_type", projectType).Debug("Detected Python project type")

	issues, err := a.runPylint(ctx, codebase.RootPath, pythonFiles)
	if err != nil {
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

// GenerateFixSuggestions generates fix suggestions for an issue.
func (a *PylintAnalyzer) GenerateFixSuggestions(issue analysis.Issue) ([]analysis.FixSuggestion, error) {
	suggestions := []analysis.FixSuggestion{}
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
			suggestions = append(suggestions, analysis.FixSuggestion{
				Description: fix,
				Confidence:  0.85,
			})
			break
		}
	}

	return suggestions, nil
}

// CanAutoFix checks if an issue can be automatically fixed.
func (a *PylintAnalyzer) CanAutoFix(issue analysis.Issue) bool {
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

func (a *PylintAnalyzer) findPythonFiles(codebase analysis.Codebase) []string {
	pythonFiles := []string{}

	for _, file := range codebase.Files {
		if (strings.HasSuffix(file, ".py") || strings.HasSuffix(file, ".pyw")) &&
			!strings.Contains(file, "__pycache__") &&
			!strings.HasSuffix(file, ".pyc") {
			pythonFiles = append(pythonFiles, file)
		}
	}

	if len(pythonFiles) == 0 && codebase.RootPath != "" {
		if err := filepath.Walk(codebase.RootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
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

func (a *PylintAnalyzer) detectPythonProject(codebase analysis.Codebase) string {
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

	for _, file := range codebase.Files {
		baseName := filepath.Base(file)
		if projectType, exists := projectIndicators[baseName]; exists {
			if projectType == "poetry" || projectType == "pipenv" {
				return projectType
			}
		}
	}

	if codebase.RootPath != "" {
		checkOrder := []string{
			"pyproject.toml", "poetry.lock",
			"Pipfile", "Pipfile.lock",
			"setup.py", "setup.cfg",
			"environment.yml", "environment.yaml", "conda.yml", "conda.yaml",
			"requirements.txt",
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

	for _, file := range codebase.Files {
		baseName := filepath.Base(file)
		if projectType, exists := projectIndicators[baseName]; exists {
			return projectType
		}
	}

	return "standalone"
}

func (a *PylintAnalyzer) countBySeverity(issues []analysis.Issue) map[string]int {
	counts := make(map[string]int)
	for _, issue := range issues {
		counts[string(issue.Severity)]++
	}
	return counts
}

func (a *PylintAnalyzer) countByCategory(issues []analysis.Issue) map[string]int {
	counts := make(map[string]int)
	for _, issue := range issues {
		counts[string(issue.Category)]++
	}
	return counts
}

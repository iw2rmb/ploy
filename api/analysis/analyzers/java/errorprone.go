package java

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/analysis"
	"github.com/sirupsen/logrus"
)

// ErrorProneAnalyzer implements the LanguageAnalyzer interface for Java using Google Error Prone
type ErrorProneAnalyzer struct {
	config *ErrorProneConfig
	logger *logrus.Logger
}

// ErrorProneConfig contains configuration for Error Prone analyzer
type ErrorProneConfig struct {
	Enabled         bool              `json:"enabled" yaml:"enabled"`
	JavacPath       string            `json:"javac_path" yaml:"javac_path"`
	ErrorPronePath  string            `json:"errorprone_path" yaml:"errorprone_path"`
	CheckerOptions  []string          `json:"checker_options" yaml:"checker_options"`
	DisabledChecks  []string          `json:"disabled_checks" yaml:"disabled_checks"`
	SeverityMapping map[string]string `json:"severity_mapping" yaml:"severity_mapping"`
	CustomPatterns  []string          `json:"custom_patterns" yaml:"custom_patterns"`
	MaxHeapSize     string            `json:"max_heap_size" yaml:"max_heap_size"`
}

// DefaultErrorProneConfig returns the default Error Prone configuration
func DefaultErrorProneConfig() *ErrorProneConfig {
	return &ErrorProneConfig{
		Enabled:        true,
		JavacPath:      "javac",
		ErrorPronePath: "/opt/errorprone/error_prone_core.jar",
		MaxHeapSize:    "2G",
		CheckerOptions: []string{
			"-Xep:NullAway:ERROR",
			"-Xep:UnusedVariable:ERROR",
			"-Xep:UnusedMethod:WARN",
			"-Xep:EqualsIncompatibleType:ERROR",
			"-Xep:CollectionIncompatibleType:ERROR",
		},
		DisabledChecks: []string{
			"FutureReturnValueIgnored",
			"ImmutableEnumChecker",
		},
		SeverityMapping: map[string]string{
			"ERROR":      "high",
			"WARNING":    "medium",
			"SUGGESTION": "low",
			"NOTE":       "info",
		},
	}
}

// NewErrorProneAnalyzer creates a new Error Prone analyzer
func NewErrorProneAnalyzer(logger *logrus.Logger) *ErrorProneAnalyzer {
	return &ErrorProneAnalyzer{
		config: DefaultErrorProneConfig(),
		logger: logger,
	}
}

// Analyze performs Java analysis using Error Prone
func (a *ErrorProneAnalyzer) Analyze(ctx context.Context, codebase analysis.Codebase) (*analysis.LanguageAnalysisResult, error) {
	startTime := time.Now()

	result := &analysis.LanguageAnalysisResult{
		Language:  "java",
		Analyzer:  "error-prone",
		Issues:    []analysis.Issue{},
		StartTime: startTime,
		Success:   true,
	}

	// Find Java files
	javaFiles := a.findJavaFiles(codebase)
	if len(javaFiles) == 0 {
		result.EndTime = time.Now()
		return result, nil
	}

	// Detect build system
	buildSystem := a.detectBuildSystem(codebase.RootPath)

	// Run Error Prone based on build system
	var issues []analysis.Issue
	var err error

	switch buildSystem {
	case "maven":
		issues, err = a.analyzeMavenProject(ctx, codebase.RootPath)
	case "gradle":
		issues, err = a.analyzeGradleProject(ctx, codebase.RootPath)
	default:
		issues, err = a.analyzeStandalone(ctx, codebase.RootPath, javaFiles)
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	}

	result.Issues = issues
	result.EndTime = time.Now()

	// Calculate metrics
	result.Metrics = analysis.AnalysisMetrics{
		TotalFiles:       len(javaFiles),
		AnalyzedFiles:    len(javaFiles),
		TotalIssues:      len(issues),
		IssuesBySeverity: a.countBySeverity(issues),
		IssuesByCategory: a.countByCategory(issues),
		AnalysisTime:     time.Since(startTime),
	}

	return result, nil
}

// GetSupportedFileTypes returns supported file extensions
func (a *ErrorProneAnalyzer) GetSupportedFileTypes() []string {
	return []string{".java"}
}

// GetAnalyzerInfo returns analyzer information
func (a *ErrorProneAnalyzer) GetAnalyzerInfo() analysis.AnalyzerInfo {
	return analysis.AnalyzerInfo{
		Name:        "error-prone",
		Version:     "2.23.0",
		Language:    "java",
		Description: "Google Error Prone static analysis for Java",
		Capabilities: []string{
			"bug-detection",
			"null-safety",
			"type-safety",
			"concurrency-bugs",
			"performance-issues",
			"security-vulnerabilities",
			"code-style",
			"custom-patterns",
		},
	}
}

// ValidateConfiguration validates the analyzer configuration
func (a *ErrorProneAnalyzer) ValidateConfiguration(config interface{}) error {
	epConfig, ok := config.(*ErrorProneConfig)
	if !ok {
		// Try to convert from map
		if mapConfig, ok := config.(map[string]interface{}); ok {
			epConfig = &ErrorProneConfig{}
			if err := a.mapToConfig(mapConfig, epConfig); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("invalid configuration type")
		}
	}

	// Check if Error Prone JAR exists
	if epConfig.ErrorPronePath != "" {
		if _, err := os.Stat(epConfig.ErrorPronePath); os.IsNotExist(err) {
			return fmt.Errorf("Error Prone JAR not found at %s", epConfig.ErrorPronePath)
		}
	}

	return nil
}

// Configure configures the analyzer
func (a *ErrorProneAnalyzer) Configure(config interface{}) error {
	if err := a.ValidateConfiguration(config); err != nil {
		return err
	}

	if epConfig, ok := config.(*ErrorProneConfig); ok {
		a.config = epConfig
	} else if mapConfig, ok := config.(map[string]interface{}); ok {
		epConfig := &ErrorProneConfig{}
		if err := a.mapToConfig(mapConfig, epConfig); err != nil {
			return err
		}
		a.config = epConfig
	}

	return nil
}

// GenerateFixSuggestions generates fix suggestions for an issue
func (a *ErrorProneAnalyzer) GenerateFixSuggestions(issue analysis.Issue) ([]analysis.FixSuggestion, error) {
	suggestions := []analysis.FixSuggestion{}

	// Map Error Prone patterns to fixes
	fixPatterns := map[string]string{
		"NullAway":                   "Add @Nullable annotation or null check",
		"UnusedVariable":             "Remove unused variable declaration",
		"UnusedMethod":               "Remove unused method or mark with @SuppressWarnings",
		"EqualsIncompatibleType":     "Fix type mismatch in equals comparison",
		"CollectionIncompatibleType": "Fix type parameter in collection operation",
		"MissingOverride":            "Add @Override annotation",
		"DefaultCharset":             "Specify explicit charset",
		"DoubleCheckedLocking":       "Use volatile or proper synchronization",
	}

	for pattern, fix := range fixPatterns {
		if strings.Contains(issue.RuleName, pattern) {
			suggestion := analysis.FixSuggestion{
				Description: fix,
				Confidence:  0.8,
				ARFRecipe:   fmt.Sprintf("org.openrewrite.java.cleanup.%s", pattern),
			}
			suggestions = append(suggestions, suggestion)
			break
		}
	}

	return suggestions, nil
}

// CanAutoFix checks if an issue can be automatically fixed
func (a *ErrorProneAnalyzer) CanAutoFix(issue analysis.Issue) bool {
	// Error Prone can suggest fixes for many patterns
	autoFixablePatterns := []string{
		"UnusedVariable",
		"UnusedMethod",
		"MissingOverride",
		"DefaultCharset",
		"UnnecessaryParentheses",
		"EmptyBlock",
		"FallThrough",
	}

	for _, pattern := range autoFixablePatterns {
		if strings.Contains(issue.RuleName, pattern) {
			return true
		}
	}

	return false
}

// GetARFRecipes returns ARF recipes for an issue
func (a *ErrorProneAnalyzer) GetARFRecipes(issue analysis.Issue) []string {
	recipes := []string{}

	// Map Error Prone checks to OpenRewrite recipes
	recipeMap := map[string][]string{
		"NullAway": {
			"org.openrewrite.java.cleanup.ExplicitInitialization",
			"org.openrewrite.java.cleanup.UseObjectNotifyAll",
		},
		"UnusedVariable": {
			"org.openrewrite.java.cleanup.RemoveUnusedLocalVariables",
		},
		"UnusedMethod": {
			"org.openrewrite.java.cleanup.RemoveUnusedPrivateMethods",
		},
		"EqualsIncompatibleType": {
			"org.openrewrite.java.cleanup.EqualsAvoidsNull",
		},
		"MissingOverride": {
			"org.openrewrite.java.cleanup.MissingOverrideAnnotation",
		},
		"DefaultCharset": {
			"org.openrewrite.java.cleanup.DefaultCharset",
		},
		"EmptyBlock": {
			"org.openrewrite.java.cleanup.EmptyBlock",
		},
	}

	for pattern, recipeList := range recipeMap {
		if strings.Contains(issue.RuleName, pattern) {
			recipes = append(recipes, recipeList...)
		}
	}

	return recipes
}

// findJavaFiles finds all Java files in the codebase
func (a *ErrorProneAnalyzer) findJavaFiles(codebase analysis.Codebase) []string {
	javaFiles := []string{}

	for _, file := range codebase.Files {
		if strings.HasSuffix(file, ".java") {
			javaFiles = append(javaFiles, file)
		}
	}

	// If no files provided, walk the directory
	if len(javaFiles) == 0 && codebase.RootPath != "" {
		filepath.Walk(codebase.RootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if strings.HasSuffix(path, ".java") {
				javaFiles = append(javaFiles, path)
			}
			return nil
		})
	}

	return javaFiles
}

// detectBuildSystem detects the build system used
func (a *ErrorProneAnalyzer) detectBuildSystem(rootPath string) string {
	if _, err := os.Stat(filepath.Join(rootPath, "pom.xml")); err == nil {
		return "maven"
	}

	if _, err := os.Stat(filepath.Join(rootPath, "build.gradle")); err == nil {
		return "gradle"
	}

	if _, err := os.Stat(filepath.Join(rootPath, "build.gradle.kts")); err == nil {
		return "gradle"
	}

	return "standalone"
}

// analyzeMavenProject analyzes a Maven project
func (a *ErrorProneAnalyzer) analyzeMavenProject(ctx context.Context, rootPath string) ([]analysis.Issue, error) {
	// Configure Maven to use Error Prone
	cmd := exec.CommandContext(ctx, "mvn", "compile",
		"-Dmaven.compiler.failOnError=false",
		fmt.Sprintf("-Dmaven.compiler.compilerArgs=-XDcompilePolicy=simple -processorpath %s", a.config.ErrorPronePath),
	)
	cmd.Dir = rootPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Parse output even if compilation fails (Error Prone reports issues as compilation errors)
		return a.parseErrorProneOutput(string(output)), nil
	}

	return a.parseErrorProneOutput(string(output)), nil
}

// analyzeGradleProject analyzes a Gradle project
func (a *ErrorProneAnalyzer) analyzeGradleProject(ctx context.Context, rootPath string) ([]analysis.Issue, error) {
	// Configure Gradle to use Error Prone (requires Error Prone plugin)
	cmd := exec.CommandContext(ctx, "./gradlew", "compileJava",
		"--continue",
		"-PerrorProneJar="+a.config.ErrorPronePath,
	)
	cmd.Dir = rootPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Parse output even if compilation fails
		return a.parseErrorProneOutput(string(output)), nil
	}

	return a.parseErrorProneOutput(string(output)), nil
}

// analyzeStandalone analyzes standalone Java files
func (a *ErrorProneAnalyzer) analyzeStandalone(ctx context.Context, rootPath string, files []string) ([]analysis.Issue, error) {
	// Build javac command with Error Prone
	args := []string{
		"-J-Xmx" + a.config.MaxHeapSize,
		"-XDcompilePolicy=simple",
		"-processorpath", a.config.ErrorPronePath,
		"-Xplugin:ErrorProne",
	}

	// Add checker options
	args = append(args, a.config.CheckerOptions...)

	// Add disabled checks
	for _, check := range a.config.DisabledChecks {
		args = append(args, "-Xep:"+check+":OFF")
	}

	// Add source files
	args = append(args, files...)

	cmd := exec.CommandContext(ctx, a.config.JavacPath, args...)
	cmd.Dir = rootPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Parse output even if compilation fails
		return a.parseErrorProneOutput(string(output)), nil
	}

	return a.parseErrorProneOutput(string(output)), nil
}

// parseErrorProneOutput parses Error Prone output into issues
func (a *ErrorProneAnalyzer) parseErrorProneOutput(output string) []analysis.Issue {
	issues := []analysis.Issue{}

	// Error Prone output format: file.java:line: severity: [CheckName] message
	pattern := regexp.MustCompile(`([^:]+):(\d+):\s*(\w+):\s*\[([^\]]+)\]\s*(.+)`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	issueID := 0

	for scanner.Scan() {
		line := scanner.Text()
		matches := pattern.FindStringSubmatch(line)

		if len(matches) == 6 {
			file := matches[1]
			lineNum := 0
			fmt.Sscanf(matches[2], "%d", &lineNum)
			severity := matches[3]
			checkName := matches[4]
			message := matches[5]

			issue := analysis.Issue{
				ID:            fmt.Sprintf("ep-%d", issueID),
				Severity:      a.mapSeverity(severity),
				Category:      a.categorizeCheck(checkName),
				RuleName:      checkName,
				Message:       message,
				File:          file,
				Line:          lineNum,
				ARFCompatible: a.CanAutoFix(analysis.Issue{RuleName: checkName}),
			}

			issues = append(issues, issue)
			issueID++
		}
	}

	return issues
}

// mapSeverity maps Error Prone severity to analysis severity
func (a *ErrorProneAnalyzer) mapSeverity(epSeverity string) analysis.SeverityLevel {
	if mapped, ok := a.config.SeverityMapping[epSeverity]; ok {
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
	switch strings.ToUpper(epSeverity) {
	case "ERROR":
		return analysis.SeverityHigh
	case "WARNING":
		return analysis.SeverityMedium
	case "SUGGESTION":
		return analysis.SeverityLow
	default:
		return analysis.SeverityInfo
	}
}

// categorizeCheck categorizes an Error Prone check
func (a *ErrorProneAnalyzer) categorizeCheck(checkName string) analysis.IssueCategory {
	// Categorize based on check name patterns
	categories := map[string]analysis.IssueCategory{
		"Null":       analysis.CategoryBug,
		"Unused":     analysis.CategoryMaintenance,
		"Security":   analysis.CategorySecurity,
		"Thread":     analysis.CategoryBug,
		"Immutable":  analysis.CategoryBug,
		"Collection": analysis.CategoryBug,
		"Equals":     analysis.CategoryBug,
		"Override":   analysis.CategoryStyle,
		"Deprecated": analysis.CategoryDeprecation,
	}

	for pattern, category := range categories {
		if strings.Contains(checkName, pattern) {
			return category
		}
	}

	return analysis.CategoryBug
}

// countBySeverity counts issues by severity
func (a *ErrorProneAnalyzer) countBySeverity(issues []analysis.Issue) map[string]int {
	counts := make(map[string]int)
	for _, issue := range issues {
		counts[string(issue.Severity)]++
	}
	return counts
}

// countByCategory counts issues by category
func (a *ErrorProneAnalyzer) countByCategory(issues []analysis.Issue) map[string]int {
	counts := make(map[string]int)
	for _, issue := range issues {
		counts[string(issue.Category)]++
	}
	return counts
}

// mapToConfig converts a map to ErrorProneConfig
func (a *ErrorProneAnalyzer) mapToConfig(m map[string]interface{}, config *ErrorProneConfig) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, config)
}

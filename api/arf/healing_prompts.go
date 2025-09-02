package arf

import (
	"fmt"
	"regexp"
	"strings"
)

// HealingPromptTemplates contains templates for different error scenarios
type HealingPromptTemplates struct {
	CompilationErrorPrompt string
	TestFailurePrompt      string
	ImportErrorPrompt      string
	DependencyPrompt       string
	RuntimeErrorPrompt     string
	LintingPrompt          string
	SecurityPrompt         string
}

// GetDefaultPromptTemplates returns default healing prompt templates
func GetDefaultPromptTemplates() *HealingPromptTemplates {
	return &HealingPromptTemplates{
		CompilationErrorPrompt: compilationErrorTemplate,
		TestFailurePrompt:      testFailureTemplate,
		ImportErrorPrompt:      importErrorTemplate,
		DependencyPrompt:       dependencyTemplate,
		RuntimeErrorPrompt:     runtimeErrorTemplate,
		LintingPrompt:          lintingTemplate,
		SecurityPrompt:         securityTemplate,
	}
}

const compilationErrorTemplate = `You are an expert {{.Language}} developer specializing in compilation error resolution.

## Error Context
- **Error Type**: Compilation Error
- **Language**: {{.Language}}
- **Source File**: {{.SourceFile}}
- **Line Number**: {{.LineNumber}}
- **Project Type**: {{.ProjectType}}

## Error Details
{{.ErrorMessage}}

## Additional Context
{{.AdditionalContext}}

## Task
Analyze this compilation error and provide a comprehensive fix.

## Required Output Format (JSON)
{
  "root_cause": "Brief explanation of why this error occurred",
  "suggested_fix": "Primary fix with actual code that should be added/modified",
  "code_changes": [
    {
      "file": "path/to/file",
      "line": 10,
      "original": "original code line",
      "fixed": "fixed code line"
    }
  ],
  "alternative_fixes": [
    "Alternative solution 1",
    "Alternative solution 2"
  ],
  "dependencies_required": ["package1", "package2"],
  "imports_required": ["import statement 1", "import statement 2"],
  "risk_assessment": "low|medium|high",
  "confidence_score": 0.95,
  "openrewrite_recipe": "org.openrewrite.java.recipe.name",
  "explanation": "Detailed explanation of the fix"
}

## Guidelines
1. Provide actual, compilable code in suggested_fix
2. Include all necessary imports
3. Consider backward compatibility
4. Suggest the safest fix as primary solution
5. Include OpenRewrite recipe if applicable`

const testFailureTemplate = `You are an expert {{.Language}} developer specializing in test failure analysis.

## Error Context
- **Error Type**: Test Failure
- **Language**: {{.Language}}
- **Test File**: {{.SourceFile}}
- **Test Method**: {{.TestMethod}}
- **Framework**: {{.TestFramework}}

## Failure Details
{{.ErrorMessage}}

## Expected vs Actual
- **Expected**: {{.ExpectedValue}}
- **Actual**: {{.ActualValue}}

## Stack Trace
{{.StackTrace}}

## Task
Analyze this test failure and determine the best resolution approach.

## Required Output Format (JSON)
{
  "failure_reason": "Why the test is failing",
  "fix_location": "test|implementation|both",
  "suggested_fix": "Primary fix with code",
  "code_changes": [
    {
      "file": "path/to/file",
      "line": 10,
      "original": "original code",
      "fixed": "fixed code"
    }
  ],
  "test_updates": "Updates needed in test if applicable",
  "implementation_updates": "Updates needed in implementation if applicable",
  "alternative_fixes": ["Alternative 1", "Alternative 2"],
  "risk_assessment": "low|medium|high",
  "confidence_score": 0.85,
  "business_impact": "Description of business logic impact",
  "regression_risk": "low|medium|high"
}

## Guidelines
1. Determine if the issue is in test expectations or implementation
2. Consider if requirements have changed
3. Check for timing/race condition issues
4. Validate test data assumptions
5. Suggest most appropriate fix location`

const importErrorTemplate = `You are an expert {{.Language}} developer specializing in dependency management.

## Error Context
- **Error Type**: Import/Module Error
- **Language**: {{.Language}}
- **Source File**: {{.SourceFile}}
- **Build Tool**: {{.BuildTool}}

## Error Details
{{.ErrorMessage}}

## Missing Import/Module
{{.MissingImport}}

## Task
Resolve the import/module error with proper dependency management.

## Required Output Format (JSON)
{
  "missing_component": "Name of missing module/package",
  "suggested_fix": "Primary resolution approach",
  "import_statement": "Correct import statement",
  "dependency_declaration": {
    "build_file": "pom.xml|build.gradle|package.json|go.mod",
    "dependency": "Full dependency declaration"
  },
  "installation_command": "Command to install dependency",
  "alternative_packages": ["alternative1", "alternative2"],
  "version_recommendation": "Recommended version",
  "compatibility_notes": "Any compatibility concerns",
  "risk_assessment": "low|medium|high",
  "confidence_score": 0.9
}

## Guidelines
1. Suggest official/well-maintained packages
2. Consider version compatibility
3. Check for security vulnerabilities
4. Prefer stable versions over latest
5. Include full dependency configuration`

const dependencyTemplate = `You are an expert in dependency resolution and version management.

## Error Context
- **Error Type**: Dependency Conflict
- **Language**: {{.Language}}
- **Build Tool**: {{.BuildTool}}
- **Project Type**: {{.ProjectType}}

## Conflict Details
{{.ErrorMessage}}

## Current Dependencies
{{.CurrentDependencies}}

## Task
Resolve the dependency conflict while maintaining stability.

## Required Output Format (JSON)
{
  "conflict_type": "version|missing|incompatible",
  "root_cause": "Explanation of the conflict",
  "suggested_fix": "Primary resolution strategy",
  "dependency_updates": [
    {
      "package": "package-name",
      "current_version": "1.0.0",
      "suggested_version": "1.1.0",
      "reason": "Why this version"
    }
  ],
  "build_file_changes": "Specific changes to build files",
  "exclusions_needed": ["Packages to exclude"],
  "alternative_solutions": ["Alternative 1", "Alternative 2"],
  "breaking_changes": ["Potential breaking change 1"],
  "migration_steps": ["Step 1", "Step 2"],
  "risk_assessment": "low|medium|high",
  "confidence_score": 0.8
}

## Guidelines
1. Minimize version changes
2. Prefer backward compatible updates
3. Consider transitive dependencies
4. Check for known vulnerabilities
5. Test compatibility thoroughly`

const runtimeErrorTemplate = `You are an expert {{.Language}} developer specializing in runtime error diagnosis.

## Error Context
- **Error Type**: Runtime Error
- **Language**: {{.Language}}
- **Environment**: {{.Environment}}
- **Error Location**: {{.SourceFile}}:{{.LineNumber}}

## Error Details
{{.ErrorMessage}}

## Stack Trace
{{.StackTrace}}

## Runtime Context
- **Memory Usage**: {{.MemoryUsage}}
- **CPU Usage**: {{.CPUUsage}}
- **Concurrent Operations**: {{.ConcurrentOps}}

## Task
Diagnose and fix the runtime error.

## Required Output Format (JSON)
{
  "error_category": "null_pointer|type_error|memory|concurrency|io|other",
  "root_cause": "Detailed explanation",
  "suggested_fix": "Primary fix with code",
  "code_changes": [
    {
      "file": "path/to/file",
      "line": 10,
      "original": "problematic code",
      "fixed": "fixed code with safety checks"
    }
  ],
  "defensive_measures": ["Validation 1", "Safety check 2"],
  "error_handling": "Proper error handling code",
  "alternative_fixes": ["Alternative 1", "Alternative 2"],
  "performance_impact": "Description of performance implications",
  "risk_assessment": "low|medium|high",
  "confidence_score": 0.75,
  "monitoring_recommendations": ["Metric to monitor", "Log to add"]
}

## Guidelines
1. Add proper null/nil checks
2. Implement defensive programming
3. Add appropriate error handling
4. Consider edge cases
5. Suggest monitoring for production`

const lintingTemplate = `You are an expert in code quality and {{.Language}} best practices.

## Error Context
- **Error Type**: Linting/Style Issue
- **Language**: {{.Language}}
- **Linter**: {{.Linter}}
- **Source File**: {{.SourceFile}}

## Linting Issues
{{.ErrorMessage}}

## Code Context
{{.CodeContext}}

## Task
Fix linting issues while maintaining code functionality.

## Required Output Format (JSON)
{
  "issue_category": "style|complexity|security|performance|maintainability",
  "issues_found": [
    {
      "rule": "rule-name",
      "severity": "error|warning|info",
      "description": "What the issue is"
    }
  ],
  "suggested_fix": "All fixes combined",
  "code_changes": [
    {
      "file": "path/to/file",
      "line": 10,
      "original": "original code",
      "fixed": "properly formatted code"
    }
  ],
  "auto_fixable": true,
  "linter_command": "Command to auto-fix",
  "suppression_option": "How to suppress if needed",
  "risk_assessment": "low",
  "confidence_score": 0.95
}

## Guidelines
1. Maintain code functionality
2. Follow language idioms
3. Consider readability
4. Don't over-optimize
5. Explain complex fixes`

const securityTemplate = `You are a security expert specializing in {{.Language}} security vulnerabilities.

## Error Context
- **Error Type**: Security Vulnerability
- **Language**: {{.Language}}
- **Severity**: {{.Severity}}
- **Source File**: {{.SourceFile}}

## Vulnerability Details
{{.ErrorMessage}}

## Security Scanner Output
{{.ScannerOutput}}

## Task
Fix the security vulnerability while maintaining functionality.

## Required Output Format (JSON)
{
  "vulnerability_type": "injection|xss|authentication|authorization|crypto|other",
  "cve_id": "CVE-XXXX-XXXXX if applicable",
  "severity": "critical|high|medium|low",
  "exploitability": "Description of how it could be exploited",
  "suggested_fix": "Secure implementation",
  "code_changes": [
    {
      "file": "path/to/file",
      "line": 10,
      "vulnerable": "vulnerable code",
      "secure": "secure code"
    }
  ],
  "security_libraries": ["Library to use for secure implementation"],
  "validation_required": ["Input validation needed"],
  "alternative_fixes": ["Alternative secure approach"],
  "compliance_impact": "PCI|HIPAA|GDPR|SOC2 implications",
  "risk_assessment": "high",
  "confidence_score": 0.9,
  "testing_approach": "How to verify the fix"
}

## Guidelines
1. Never compromise security for functionality
2. Use established security libraries
3. Apply defense in depth
4. Consider the full attack surface
5. Include security test cases`

// GeneratePrompt generates a specific prompt based on error type and context
func GeneratePrompt(errorType, language string, context map[string]string) string {
	templates := GetDefaultPromptTemplates()

	var template string
	switch strings.ToLower(errorType) {
	case "compilation":
		template = templates.CompilationErrorPrompt
	case "test":
		template = templates.TestFailurePrompt
	case "import":
		template = templates.ImportErrorPrompt
	case "dependency":
		template = templates.DependencyPrompt
	case "runtime":
		template = templates.RuntimeErrorPrompt
	case "linting", "style":
		template = templates.LintingPrompt
	case "security":
		template = templates.SecurityPrompt
	default:
		// Use compilation template as default
		template = templates.CompilationErrorPrompt
	}

	// Replace placeholders
	result := template
	for key, value := range context {
		placeholder := fmt.Sprintf("{{.%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// Clean up any remaining placeholders
	remaining := regexp.MustCompile(`\{\{\.[\w]+\}\}`)
	result = remaining.ReplaceAllString(result, "Not available")

	return result
}

// BuildPromptContext builds context map from error details
func BuildPromptContext(errorContext ErrorContext, language string) map[string]string {
	context := map[string]string{
		"Language":     language,
		"ErrorMessage": errorContext.ErrorMessage,
		"ErrorType":    errorContext.ErrorType,
		"SourceFile":   errorContext.SourceFile,
		"LineNumber":   "Unknown",
	}

	// Extract line number from error details if available
	if errorContext.ErrorDetails != nil {
		if lineNum, ok := errorContext.ErrorDetails["line_number"]; ok {
			context["LineNumber"] = lineNum
		}
	}

	// Add language-specific context
	switch language {
	case "java":
		context["BuildTool"] = "Maven/Gradle"
		context["TestFramework"] = "JUnit"
		context["ProjectType"] = "Java Application"

	case "python":
		context["BuildTool"] = "pip/poetry"
		context["TestFramework"] = "pytest"
		context["ProjectType"] = "Python Package"
		context["Linter"] = "pylint/flake8"

	case "go":
		context["BuildTool"] = "go mod"
		context["TestFramework"] = "go test"
		context["ProjectType"] = "Go Module"
		context["Linter"] = "golangci-lint"

	case "javascript", "typescript":
		context["BuildTool"] = "npm/yarn"
		context["TestFramework"] = "jest/mocha"
		context["ProjectType"] = "Node.js Application"
		context["Linter"] = "eslint"
	}

	// Add default values for missing context
	if context["SourceFile"] == "" {
		context["SourceFile"] = "Unknown"
	}
	if context["LineNumber"] == "" {
		context["LineNumber"] = "Unknown"
	}

	// Add additional context based on error type
	if errorContext.CompilerOutput != "" {
		context["AdditionalContext"] = errorContext.CompilerOutput
	} else {
		context["AdditionalContext"] = "No additional context available"
	}

	return context
}

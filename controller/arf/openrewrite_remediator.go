package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// OpenRewriteRemediator implements VulnerabilityRemediator using OpenRewrite recipes
type OpenRewriteRemediator struct {
	openrewritePath string
	recipesPath     string
	templatePath    string
}

// NewOpenRewriteRemediator creates a new OpenRewrite-based remediator
func NewOpenRewriteRemediator() *OpenRewriteRemediator {
	return &OpenRewriteRemediator{
		openrewritePath: "java", // OpenRewrite runs via JVM
		recipesPath:     "/opt/ploy/recipes",
		templatePath:    "/opt/ploy/templates/security",
	}
}

// GenerateRemediation creates a remediation recipe for a vulnerability
func (o *OpenRewriteRemediator) GenerateRemediation(vuln VulnerabilityInfo, codebase Codebase) (*RemediationRecipe, error) {
	// Determine remediation strategy based on vulnerability type and package ecosystem
	strategy := o.determineStrategy(vuln)
	
	switch strategy {
	case "dependency_upgrade":
		return o.generateDependencyUpgradeRecipe(vuln, codebase)
	case "code_transformation":
		return o.generateCodeTransformationRecipe(vuln, codebase)
	case "configuration_fix":
		return o.generateConfigurationFixRecipe(vuln, codebase)
	case "security_hardening":
		return o.generateSecurityHardeningRecipe(vuln, codebase)
	default:
		return o.generateGenericRecipe(vuln, codebase)
	}
}

// determineStrategy determines the best remediation strategy for a vulnerability
func (o *OpenRewriteRemediator) determineStrategy(vuln VulnerabilityInfo) string {
	cveDesc := strings.ToLower(vuln.CVE.Description)
	
	// Dependency-related vulnerabilities
	if strings.Contains(cveDesc, "dependency") ||
		strings.Contains(cveDesc, "library") ||
		strings.Contains(cveDesc, "package") ||
		vuln.HasFix {
		return "dependency_upgrade"
	}
	
	// Code-level security issues
	if strings.Contains(cveDesc, "injection") ||
		strings.Contains(cveDesc, "xss") ||
		strings.Contains(cveDesc, "deserialization") ||
		strings.Contains(cveDesc, "buffer overflow") {
		return "code_transformation"
	}
	
	// Configuration-related issues
	if strings.Contains(cveDesc, "configuration") ||
		strings.Contains(cveDesc, "misconfiguration") ||
		strings.Contains(cveDesc, "default") {
		return "configuration_fix"
	}
	
	// General security hardening
	if strings.Contains(cveDesc, "authentication") ||
		strings.Contains(cveDesc, "authorization") ||
		strings.Contains(cveDesc, "encryption") {
		return "security_hardening"
	}
	
	return "generic"
}

// generateDependencyUpgradeRecipe creates a recipe for dependency upgrades
func (o *OpenRewriteRemediator) generateDependencyUpgradeRecipe(vuln VulnerabilityInfo, codebase Codebase) (*RemediationRecipe, error) {
	recipeID := fmt.Sprintf("dep-upgrade-%s-%d", vuln.CVE.ID, time.Now().Unix())
	
	var operations []RemediationOperation
	
	// Determine package manager and build files
	buildFiles := o.findBuildFiles(codebase)
	
	for _, buildFile := range buildFiles {
		op := RemediationOperation{
			Action: "upgrade",
			Target: OperationTarget{
				Type:       "dependency",
				Identifier: vuln.Package.Name,
				Path:       buildFile,
			},
			Parameters: map[string]interface{}{
				"from_version": vuln.Package.Version,
				"to_version":   vuln.FixVersion,
				"ecosystem":    vuln.Package.Ecosystem,
			},
			Conditions: []OperationCondition{
				{
					Type:     "version_check",
					Field:    "current_version",
					Operator: "equals",
					Value:    vuln.Package.Version,
					Required: true,
				},
			},
		}
		operations = append(operations, op)
	}
	
	// Generate OpenRewrite recipe YAML
	recipeYAML := o.generateOpenRewriteRecipeYAML(recipeID, "Dependency Upgrade", operations)
	
	recipe := &RemediationRecipe{
		ID:              recipeID,
		Name:            fmt.Sprintf("Upgrade %s to fix %s", vuln.Package.Name, vuln.CVE.ID),
		Description:     fmt.Sprintf("Upgrades %s from %s to %s to address %s", vuln.Package.Name, vuln.Package.Version, vuln.FixVersion, vuln.CVE.ID),
		Vulnerabilities: []string{vuln.CVE.ID},
		Recipe: AutoRemediationRecipe{
			Type:       "openrewrite",
			Operations: operations,
			Validation: ValidationCriteria{
				Tests: []ValidationTest{
					{
						Type:        "build",
						Command:     o.getBuildCommand(codebase),
						Expected:    0,
						Description: "Build succeeds after dependency upgrade",
						Critical:    true,
					},
					{
						Type:        "security_scan",
						Command:     fmt.Sprintf("grype dir:%s --fail-on high", codebase.Path),
						Expected:    0,
						Description: "No high or critical vulnerabilities found",
						Critical:    false,
					},
				},
				Timeout:     10 * time.Minute,
				SuccessRate: 1.0,
			},
			Rollback: RollbackStrategy{
				Type:    "version_revert",
				Timeout: 5 * time.Minute,
				Steps: []RollbackStep{
					{
						Action: "revert",
						Target: OperationTarget{
							Type: "dependency",
							Identifier: vuln.Package.Name,
						},
						Parameters: map[string]interface{}{
							"to_version": vuln.Package.Version,
						},
						Order: 1,
					},
				},
			},
		},
		Metadata: map[string]interface{}{
			"openrewrite_recipe": recipeYAML,
			"strategy":          "dependency_upgrade",
			"package":           vuln.Package.Name,
			"ecosystem":         vuln.Package.Ecosystem,
		},
		CreatedAt: time.Now(),
		Version:   "1.0",
	}
	
	return recipe, nil
}

// generateCodeTransformationRecipe creates a recipe for code-level fixes
func (o *OpenRewriteRemediator) generateCodeTransformationRecipe(vuln VulnerabilityInfo, codebase Codebase) (*RemediationRecipe, error) {
	recipeID := fmt.Sprintf("code-fix-%s-%d", vuln.CVE.ID, time.Now().Unix())
	
	// Determine transformation type based on vulnerability
	transformations := o.determineCodeTransformations(vuln)
	
	var operations []RemediationOperation
	for _, transform := range transformations {
		op := RemediationOperation{
			Action: "transform",
			Target: OperationTarget{
				Type:    "file",
				Pattern: transform.FilePattern,
			},
			Parameters: map[string]interface{}{
				"transformation": transform.Recipe,
				"search_pattern": transform.SearchPattern,
				"replacement":    transform.Replacement,
			},
		}
		operations = append(operations, op)
	}
	
	recipe := &RemediationRecipe{
		ID:              recipeID,
		Name:            fmt.Sprintf("Code transformation for %s", vuln.CVE.ID),
		Description:     fmt.Sprintf("Applies code transformations to fix %s: %s", vuln.CVE.ID, vuln.CVE.Description),
		Vulnerabilities: []string{vuln.CVE.ID},
		Recipe: AutoRemediationRecipe{
			Type:       "openrewrite",
			Operations: operations,
			Validation: ValidationCriteria{
				Tests: []ValidationTest{
					{
						Type:        "build",
						Command:     o.getBuildCommand(codebase),
						Expected:    0,
						Description: "Build succeeds after code transformation",
						Critical:    true,
					},
					{
						Type:        "static_analysis",
						Command:     "spotbugs",
						Expected:    "no_high_priority_bugs",
						Description: "No high priority security bugs found",
						Critical:    false,
					},
				},
				Timeout:     15 * time.Minute,
				SuccessRate: 0.9,
			},
			Rollback: RollbackStrategy{
				Type:    "git_revert",
				Timeout: 5 * time.Minute,
				Steps: []RollbackStep{
					{
						Action: "git_reset",
						Parameters: map[string]interface{}{
							"ref": "HEAD~1",
						},
						Order: 1,
					},
				},
			},
		},
		Metadata: map[string]interface{}{
			"strategy":        "code_transformation",
			"transformations": transformations,
		},
		CreatedAt: time.Now(),
		Version:   "1.0",
	}
	
	return recipe, nil
}

// generateConfigurationFixRecipe creates a recipe for configuration fixes
func (o *OpenRewriteRemediator) generateConfigurationFixRecipe(vuln VulnerabilityInfo, codebase Codebase) (*RemediationRecipe, error) {
	recipeID := fmt.Sprintf("config-fix-%s-%d", vuln.CVE.ID, time.Now().Unix())
	
	configFixes := o.determineConfigurationFixes(vuln, codebase)
	
	var operations []RemediationOperation
	for _, fix := range configFixes {
		op := RemediationOperation{
			Action: "configure",
			Target: OperationTarget{
				Type: "configuration",
				Path: fix.ConfigFile,
			},
			Parameters: map[string]interface{}{
				"key":   fix.Key,
				"value": fix.Value,
				"type":  fix.Type,
			},
		}
		operations = append(operations, op)
	}
	
	recipe := &RemediationRecipe{
		ID:              recipeID,
		Name:            fmt.Sprintf("Configuration fix for %s", vuln.CVE.ID),
		Description:     fmt.Sprintf("Applies configuration changes to fix %s", vuln.CVE.ID),
		Vulnerabilities: []string{vuln.CVE.ID},
		Recipe: AutoRemediationRecipe{
			Type:       "configuration",
			Operations: operations,
			Validation: ValidationCriteria{
				Tests: []ValidationTest{
					{
						Type:        "config_validation",
						Description: "Configuration is valid",
						Critical:    true,
					},
					{
						Type:        "integration_test",
						Command:     "npm test",
						Expected:    0,
						Description: "Integration tests pass",
						Critical:    false,
					},
				},
				Timeout:     5 * time.Minute,
				SuccessRate: 1.0,
			},
			Rollback: RollbackStrategy{
				Type:    "config_revert",
				Timeout: 2 * time.Minute,
			},
		},
		CreatedAt: time.Now(),
		Version:   "1.0",
	}
	
	return recipe, nil
}

// generateSecurityHardeningRecipe creates a recipe for security hardening
func (o *OpenRewriteRemediator) generateSecurityHardeningRecipe(vuln VulnerabilityInfo, codebase Codebase) (*RemediationRecipe, error) {
	recipeID := fmt.Sprintf("hardening-%s-%d", vuln.CVE.ID, time.Now().Unix())
	
	hardeningMeasures := o.determineSecurityHardening(vuln, codebase)
	
	var operations []RemediationOperation
	for _, measure := range hardeningMeasures {
		op := RemediationOperation{
			Action: measure.Action,
			Target: OperationTarget{
				Type: measure.TargetType,
				Path: measure.Path,
			},
			Parameters: measure.Parameters,
		}
		operations = append(operations, op)
	}
	
	recipe := &RemediationRecipe{
		ID:              recipeID,
		Name:            fmt.Sprintf("Security hardening for %s", vuln.CVE.ID),
		Description:     fmt.Sprintf("Applies security hardening measures to address %s", vuln.CVE.ID),
		Vulnerabilities: []string{vuln.CVE.ID},
		Recipe: AutoRemediationRecipe{
			Type:       "security_hardening",
			Operations: operations,
			Validation: ValidationCriteria{
				Tests: []ValidationTest{
					{
						Type:        "security_test",
						Description: "Security tests pass",
						Critical:    true,
					},
				},
				Timeout:     10 * time.Minute,
				SuccessRate: 0.95,
			},
		},
		CreatedAt: time.Now(),
		Version:   "1.0",
	}
	
	return recipe, nil
}

// generateGenericRecipe creates a generic remediation recipe
func (o *OpenRewriteRemediator) generateGenericRecipe(vuln VulnerabilityInfo, codebase Codebase) (*RemediationRecipe, error) {
	recipeID := fmt.Sprintf("generic-%s-%d", vuln.CVE.ID, time.Now().Unix())
	
	recipe := &RemediationRecipe{
		ID:              recipeID,
		Name:            fmt.Sprintf("Generic remediation for %s", vuln.CVE.ID),
		Description:     fmt.Sprintf("Manual remediation required for %s: %s", vuln.CVE.ID, vuln.CVE.Description),
		Vulnerabilities: []string{vuln.CVE.ID},
		Recipe: AutoRemediationRecipe{
			Type: "manual",
			Operations: []RemediationOperation{
				{
					Action: "manual_review",
					Target: OperationTarget{
						Type: "codebase",
						Path: codebase.Path,
					},
					Parameters: map[string]interface{}{
						"instructions": vuln.CVE.Remediation.Instructions,
						"references":   vuln.CVE.References,
					},
				},
			},
			Validation: ValidationCriteria{
				Tests: []ValidationTest{
					{
						Type:        "manual_verification",
						Description: "Manual verification required",
						Critical:    true,
					},
				},
				Timeout:     30 * time.Minute,
				SuccessRate: 1.0,
			},
		},
		Metadata: map[string]interface{}{
			"strategy": "manual",
			"requires_human_intervention": true,
		},
		CreatedAt: time.Now(),
		Version:   "1.0",
	}
	
	return recipe, nil
}

// ValidateRemediation validates that a remediation recipe is well-formed and safe
func (o *OpenRewriteRemediator) ValidateRemediation(recipe *RemediationRecipe) error {
	if recipe.ID == "" {
		return fmt.Errorf("recipe ID is required")
	}
	
	if len(recipe.Recipe.Operations) == 0 {
		return fmt.Errorf("recipe must have at least one operation")
	}
	
	// Validate each operation
	for i, op := range recipe.Recipe.Operations {
		if err := o.validateOperation(op); err != nil {
			return fmt.Errorf("operation %d is invalid: %w", i, err)
		}
	}
	
	// Validate that rollback strategy exists for non-manual recipes
	if recipe.Recipe.Type != "manual" && recipe.Recipe.Rollback.Type == "" {
		return fmt.Errorf("rollback strategy is required for automated recipes")
	}
	
	return nil
}

// validateOperation validates a single remediation operation
func (o *OpenRewriteRemediator) validateOperation(op RemediationOperation) error {
	if op.Action == "" {
		return fmt.Errorf("operation action is required")
	}
	
	if op.Target.Type == "" {
		return fmt.Errorf("operation target type is required")
	}
	
	// Validate action-specific requirements
	switch op.Action {
	case "upgrade":
		if _, ok := op.Parameters["to_version"]; !ok {
			return fmt.Errorf("upgrade action requires 'to_version' parameter")
		}
	case "transform":
		if _, ok := op.Parameters["transformation"]; !ok {
			return fmt.Errorf("transform action requires 'transformation' parameter")
		}
	case "configure":
		if _, ok := op.Parameters["key"]; !ok {
			return fmt.Errorf("configure action requires 'key' parameter")
		}
		if _, ok := op.Parameters["value"]; !ok {
			return fmt.Errorf("configure action requires 'value' parameter")
		}
	}
	
	return nil
}

// ApplyRemediation applies a remediation recipe in a sandbox environment
func (o *OpenRewriteRemediator) ApplyRemediation(ctx context.Context, recipe *RemediationRecipe, sandbox string) (*RemediationResult, error) {
	startTime := time.Now()
	result := &RemediationResult{
		VulnerabilitiesFixed: []string{},
		Errors:              []string{},
		Warnings:            []string{},
		ValidationResults:   []ValidationResult{},
		ChangeSummary: ChangeSummary{
			FilesModified:        []string{},
			DependenciesChanged:  []DependencyChange{},
			ConfigurationChanges: []ConfigChange{},
		},
	}
	
	// Apply each operation
	for i, operation := range recipe.Recipe.Operations {
		if err := o.applyOperation(ctx, operation, sandbox, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Operation %d failed: %v", i, err))
			result.Success = false
			result.RollbackRequired = true
			break
		}
	}
	
	// Run validation tests if operations succeeded
	if len(result.Errors) == 0 {
		for _, test := range recipe.Recipe.Validation.Tests {
			validationResult := o.runValidationTest(ctx, test, sandbox)
			result.ValidationResults = append(result.ValidationResults, validationResult)
			
			if !validationResult.Success && validationResult.Critical {
				result.Success = false
				result.RollbackRequired = true
				result.Errors = append(result.Errors, fmt.Sprintf("Critical validation failed: %s", validationResult.Error))
				break
			}
		}
		
		// Check overall success rate
		successCount := 0
		for _, vr := range result.ValidationResults {
			if vr.Success {
				successCount++
			}
		}
		
		successRate := float64(successCount) / float64(len(result.ValidationResults))
		if successRate < recipe.Recipe.Validation.SuccessRate {
			result.Success = false
			result.RollbackRequired = true
			result.Errors = append(result.Errors, fmt.Sprintf("Validation success rate %.2f below required %.2f", successRate, recipe.Recipe.Validation.SuccessRate))
		}
	}
	
	// If successful, mark vulnerabilities as fixed
	if len(result.Errors) == 0 {
		result.Success = true
		result.VulnerabilitiesFixed = recipe.Vulnerabilities
	}
	
	result.Duration = time.Since(startTime)
	return result, nil
}

// applyOperation applies a single remediation operation
func (o *OpenRewriteRemediator) applyOperation(ctx context.Context, op RemediationOperation, sandbox string, result *RemediationResult) error {
	switch op.Action {
	case "upgrade":
		return o.applyDependencyUpgrade(ctx, op, sandbox, result)
	case "transform":
		return o.applyCodeTransformation(ctx, op, sandbox, result)
	case "configure":
		return o.applyConfiguration(ctx, op, sandbox, result)
	case "manual_review":
		result.Warnings = append(result.Warnings, "Manual review required - automatic application skipped")
		return nil
	default:
		return fmt.Errorf("unsupported operation action: %s", op.Action)
	}
}

// applyDependencyUpgrade applies a dependency upgrade operation
func (o *OpenRewriteRemediator) applyDependencyUpgrade(ctx context.Context, op RemediationOperation, sandbox string, result *RemediationResult) error {
	packageName := op.Target.Identifier
	toVersion, ok := op.Parameters["to_version"].(string)
	if !ok {
		return fmt.Errorf("invalid to_version parameter")
	}
	
	buildFile := filepath.Join(sandbox, op.Target.Path)
	
	// Apply upgrade based on build system
	var cmd *exec.Cmd
	if strings.Contains(op.Target.Path, "pom.xml") {
		// Maven
		cmd = exec.CommandContext(ctx, "mvn", "versions:use-dep-version",
			fmt.Sprintf("-Dincludes=%s", packageName),
			fmt.Sprintf("-DdepVersion=%s", toVersion),
			fmt.Sprintf("-f%s", buildFile))
	} else if strings.Contains(op.Target.Path, "build.gradle") {
		// Gradle - requires manual file editing or custom script
		return o.updateGradleDependency(buildFile, packageName, toVersion, result)
	} else if strings.Contains(op.Target.Path, "package.json") {
		// npm
		cmd = exec.CommandContext(ctx, "npm", "install", fmt.Sprintf("%s@%s", packageName, toVersion))
		cmd.Dir = filepath.Dir(buildFile)
	} else if strings.Contains(op.Target.Path, "go.mod") {
		// Go modules
		cmd = exec.CommandContext(ctx, "go", "get", fmt.Sprintf("%s@%s", packageName, toVersion))
		cmd.Dir = filepath.Dir(buildFile)
	}
	
	if cmd != nil {
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("dependency upgrade failed: %w, output: %s", err, string(output))
		}
	}
	
	// Record the change
	result.ChangeSummary.DependenciesChanged = append(result.ChangeSummary.DependenciesChanged, DependencyChange{
		Name:      packageName,
		ToVersion: toVersion,
		Action:    "upgrade",
		Reason:    "Security vulnerability remediation",
	})
	
	result.ChangeSummary.FilesModified = append(result.ChangeSummary.FilesModified, op.Target.Path)
	
	return nil
}

// applyCodeTransformation applies a code transformation operation
func (o *OpenRewriteRemediator) applyCodeTransformation(ctx context.Context, op RemediationOperation, sandbox string, result *RemediationResult) error {
	transformation, ok := op.Parameters["transformation"].(string)
	if !ok {
		return fmt.Errorf("invalid transformation parameter")
	}
	
	// Create OpenRewrite recipe file
	recipeContent := fmt.Sprintf(`
type: specs.openrewrite.org/v1beta/recipe
name: ploy.security.%s
displayName: Security Transformation
description: Applies security-related code transformations
recipeList:
  - %s
`, generateID(), transformation)
	
	recipeFile := filepath.Join(sandbox, "ploy-security-recipe.yml")
	if err := o.writeFile(recipeFile, recipeContent); err != nil {
		return fmt.Errorf("failed to create recipe file: %w", err)
	}
	
	// Run OpenRewrite
	cmd := exec.CommandContext(ctx, "java", "-jar", "/opt/openrewrite/rewrite-cli.jar",
		"--recipe-file", recipeFile,
		"--source-dir", sandbox)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("OpenRewrite transformation failed: %w, output: %s", err, string(output))
	}
	
	// Parse output to identify changed files
	changedFiles := o.parseOpenRewriteOutput(string(output))
	result.ChangeSummary.FilesModified = append(result.ChangeSummary.FilesModified, changedFiles...)
	
	return nil
}

// applyConfiguration applies a configuration change operation
func (o *OpenRewriteRemediator) applyConfiguration(ctx context.Context, op RemediationOperation, sandbox string, result *RemediationResult) error {
	configFile := filepath.Join(sandbox, op.Target.Path)
	key, ok := op.Parameters["key"].(string)
	if !ok {
		return fmt.Errorf("invalid key parameter")
	}
	
	value := op.Parameters["value"]
	
	// Apply configuration change based on file type
	if strings.HasSuffix(configFile, ".json") {
		return o.updateJSONConfig(configFile, key, value, result)
	} else if strings.HasSuffix(configFile, ".yaml") || strings.HasSuffix(configFile, ".yml") {
		return o.updateYAMLConfig(configFile, key, value, result)
	} else if strings.HasSuffix(configFile, ".properties") {
		return o.updatePropertiesConfig(configFile, key, value, result)
	}
	
	return fmt.Errorf("unsupported configuration file type: %s", configFile)
}

// runValidationTest runs a single validation test
func (o *OpenRewriteRemediator) runValidationTest(ctx context.Context, test ValidationTest, sandbox string) ValidationResult {
	startTime := time.Now()
	result := ValidationResult{
		Test:     test,
		Critical: test.Critical,
	}
	
	switch test.Type {
	case "build":
		cmd := exec.CommandContext(ctx, "bash", "-c", test.Command)
		cmd.Dir = sandbox
		output, err := cmd.CombinedOutput()
		result.Output = string(output)
		if err != nil {
			result.Error = err.Error()
			result.Success = false
		} else {
			result.Success = true
		}
	case "security_scan":
		cmd := exec.CommandContext(ctx, "bash", "-c", test.Command)
		cmd.Dir = sandbox
		output, err := cmd.CombinedOutput()
		result.Output = string(output)
		result.Success = err == nil
		if err != nil {
			result.Error = err.Error()
		}
	case "manual_verification":
		result.Success = false
		result.Error = "Manual verification required"
	default:
		result.Success = false
		result.Error = fmt.Sprintf("Unknown test type: %s", test.Type)
	}
	
	result.Duration = time.Since(startTime)
	return result
}

// Helper functions and data structures

type CodeTransformation struct {
	FilePattern   string
	SearchPattern string
	Replacement   string
	Recipe        string
}

type ConfigurationFix struct {
	ConfigFile string
	Key        string
	Value      interface{}
	Type       string
}

type SecurityHardening struct {
	Action     string
	TargetType string
	Path       string
	Parameters map[string]interface{}
}

// determineCodeTransformations determines appropriate code transformations
func (o *OpenRewriteRemediator) determineCodeTransformations(vuln VulnerabilityInfo) []CodeTransformation {
	var transformations []CodeTransformation
	
	cveDesc := strings.ToLower(vuln.CVE.Description)
	
	if strings.Contains(cveDesc, "sql injection") {
		transformations = append(transformations, CodeTransformation{
			FilePattern:   "**/*.java",
			SearchPattern: "Statement.*execute",
			Recipe:        "org.openrewrite.java.security.UseParameterizedQuery",
		})
	}
	
	if strings.Contains(cveDesc, "xss") || strings.Contains(cveDesc, "cross-site scripting") {
		transformations = append(transformations, CodeTransformation{
			FilePattern: "**/*.java",
			Recipe:      "org.openrewrite.java.security.XSSProtection",
		})
	}
	
	if strings.Contains(cveDesc, "insecure deserialization") {
		transformations = append(transformations, CodeTransformation{
			FilePattern: "**/*.java",
			Recipe:      "org.openrewrite.java.security.SecureDeserialization",
		})
	}
	
	return transformations
}

// determineConfigurationFixes determines appropriate configuration fixes
func (o *OpenRewriteRemediator) determineConfigurationFixes(vuln VulnerabilityInfo, codebase Codebase) []ConfigurationFix {
	var fixes []ConfigurationFix
	
	cveDesc := strings.ToLower(vuln.CVE.Description)
	
	if strings.Contains(cveDesc, "default") && strings.Contains(cveDesc, "password") {
		fixes = append(fixes, ConfigurationFix{
			ConfigFile: "application.properties",
			Key:        "spring.security.user.password",
			Value:      "${SECURE_PASSWORD}",
			Type:       "security",
		})
	}
	
	if strings.Contains(cveDesc, "tls") || strings.Contains(cveDesc, "ssl") {
		fixes = append(fixes, ConfigurationFix{
			ConfigFile: "application.yml",
			Key:        "server.ssl.enabled",
			Value:      true,
			Type:       "security",
		})
	}
	
	return fixes
}

// determineSecurityHardening determines appropriate security hardening measures
func (o *OpenRewriteRemediator) determineSecurityHardening(vuln VulnerabilityInfo, codebase Codebase) []SecurityHardening {
	var measures []SecurityHardening
	
	cveDesc := strings.ToLower(vuln.CVE.Description)
	
	if strings.Contains(cveDesc, "authentication") {
		measures = append(measures, SecurityHardening{
			Action:     "add_security_headers",
			TargetType: "configuration",
			Path:       "security-config.yml",
			Parameters: map[string]interface{}{
				"headers": []string{"X-Frame-Options", "X-Content-Type-Options", "X-XSS-Protection"},
			},
		})
	}
	
	return measures
}

// findBuildFiles finds build configuration files in the codebase
func (o *OpenRewriteRemediator) findBuildFiles(codebase Codebase) []string {
	var buildFiles []string
	
	commonBuildFiles := []string{
		"pom.xml",
		"build.gradle",
		"build.gradle.kts",
		"package.json",
		"go.mod",
		"Cargo.toml",
		"requirements.txt",
		"setup.py",
	}
	
	for _, file := range commonBuildFiles {
		path := filepath.Join(codebase.Path, file)
		if o.fileExists(path) {
			buildFiles = append(buildFiles, file)
		}
	}
	
	return buildFiles
}

// getBuildCommand returns the appropriate build command for the codebase
func (o *OpenRewriteRemediator) getBuildCommand(codebase Codebase) string {
	if o.fileExists(filepath.Join(codebase.Path, "pom.xml")) {
		return "mvn clean compile"
	}
	if o.fileExists(filepath.Join(codebase.Path, "build.gradle")) {
		return "./gradlew build"
	}
	if o.fileExists(filepath.Join(codebase.Path, "package.json")) {
		return "npm run build"
	}
	if o.fileExists(filepath.Join(codebase.Path, "go.mod")) {
		return "go build ./..."
	}
	return "echo 'No build command detected'"
}

// generateOpenRewriteRecipeYAML generates an OpenRewrite recipe YAML
func (o *OpenRewriteRemediator) generateOpenRewriteRecipeYAML(id, name string, operations []RemediationOperation) string {
	// This is a simplified version - in practice, would generate proper OpenRewrite YAML
	return fmt.Sprintf(`
type: specs.openrewrite.org/v1beta/recipe
name: ploy.security.%s
displayName: %s
description: Automated security remediation recipe
recipeList:
  - org.openrewrite.maven.UpgradeDependencyVersion
`, id, name)
}

// Utility functions
func (o *OpenRewriteRemediator) fileExists(path string) bool {
	cmd := exec.Command("test", "-f", path)
	return cmd.Run() == nil
}

func (o *OpenRewriteRemediator) writeFile(path, content string) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", path, content))
	return cmd.Run()
}

func (o *OpenRewriteRemediator) updateGradleDependency(buildFile, packageName, version string, result *RemediationResult) error {
	// Simplified Gradle dependency update using sed
	searchPattern := fmt.Sprintf("implementation.*%s.*", regexp.QuoteMeta(packageName))
	replacement := fmt.Sprintf("implementation '%s:%s'", packageName, version)
	
	cmd := exec.Command("sed", "-i", fmt.Sprintf("s/%s/%s/g", searchPattern, replacement), buildFile)
	return cmd.Run()
}

func (o *OpenRewriteRemediator) updateJSONConfig(configFile, key string, value interface{}, result *RemediationResult) error {
	// Simplified JSON update - in practice, would use proper JSON library
	cmd := exec.Command("jq", fmt.Sprintf(".%s = %v", key, value), configFile)
	return cmd.Run()
}

func (o *OpenRewriteRemediator) updateYAMLConfig(configFile, key string, value interface{}, result *RemediationResult) error {
	// Simplified YAML update - in practice, would use proper YAML library
	cmd := exec.Command("yq", "eval", fmt.Sprintf(".%s = %v", key, value), configFile)
	return cmd.Run()
}

func (o *OpenRewriteRemediator) updatePropertiesConfig(configFile, key string, value interface{}, result *RemediationResult) error {
	// Simple properties update
	line := fmt.Sprintf("%s=%v", key, value)
	cmd := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' >> %s", line, configFile))
	return cmd.Run()
}

func (o *OpenRewriteRemediator) parseOpenRewriteOutput(output string) []string {
	var changedFiles []string
	lines := strings.Split(output, "\n")
	
	for _, line := range lines {
		if strings.Contains(line, "Changed") && strings.Contains(line, ".java") {
			// Extract filename from OpenRewrite output
			if matches := regexp.MustCompile(`Changed\s+(.+\.java)`).FindStringSubmatch(line); len(matches) > 1 {
				changedFiles = append(changedFiles, matches[1])
			}
		}
	}
	
	return changedFiles
}
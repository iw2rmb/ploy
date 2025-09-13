package analysis

import (
	"context"
	"fmt"
	"time"

	"github.com/iw2rmb/ploy/api/arf"
	"github.com/iw2rmb/ploy/api/arf/models"
	recipes "github.com/iw2rmb/ploy/api/recipes"
	"github.com/sirupsen/logrus"
)

// ARFIntegration handles integration with the Automated Remediation Framework
type ARFIntegration struct {
	arfHandler     *arf.Handler
	recipeExecutor *recipes.RecipeExecutor
	sandboxManager arf.SandboxManager
	logger         *logrus.Logger
}

// NewARFIntegration creates a new ARF integration
func NewARFIntegration(arfHandler *arf.Handler, executor *recipes.RecipeExecutor, sandboxMgr arf.SandboxManager, logger *logrus.Logger) *ARFIntegration {
	return &ARFIntegration{
		arfHandler:     arfHandler,
		recipeExecutor: executor,
		sandboxManager: sandboxMgr,
		logger:         logger,
	}
}

// TriggerRemediation triggers ARF remediation for analysis issues
func (a *ARFIntegration) TriggerRemediation(ctx context.Context, result *AnalysisResult) error {
	if len(result.ARFTriggers) == 0 {
		return nil
	}

	a.logger.WithFields(logrus.Fields{
		"repository": result.Repository.Name,
		"triggers":   len(result.ARFTriggers),
	}).Info("Triggering ARF remediation")

	// Group triggers by recipe for efficient processing
	recipeGroups := a.groupTriggersByRecipe(result.ARFTriggers)

	// Process each recipe group
	for recipeName, triggers := range recipeGroups {
		if err := a.processRecipeGroup(ctx, result.Repository, recipeName, triggers); err != nil {
			a.logger.WithError(err).WithField("recipe", recipeName).
				Error("Failed to process recipe group")
			// Continue with other recipes even if one fails
		}
	}

	return nil
}

// CreateRemediationWorkflow removed - workflow functionality moved to transform command

// GenerateRecipeFromIssues generates an ARF recipe from analysis issues
func (a *ARFIntegration) GenerateRecipeFromIssues(issues []Issue, language string) (*models.Recipe, error) {
	recipe := &models.Recipe{
		Metadata: models.RecipeMetadata{
			Name:        "Static Analysis Auto-Generated Recipe",
			Description: fmt.Sprintf("Auto-generated recipe for %d issues", len(issues)),
			Version:     "1.0.0",
			Languages:   []string{language},
			Categories:  []string{"auto-remediation"},
			Tags:        []string{"static-analysis", "auto-generated"},
		},
	}

	// Group issues by type and generate steps
	steps := a.generateRecipeSteps(issues, language)
	recipe.Steps = make([]models.RecipeStep, len(steps))
	for i, step := range steps {
		recipe.Steps[i] = *step
	}

	// Set system fields
	recipe.SetSystemFields("static-analysis-engine")

	// Add validation rules
	recipe.Validation = a.generateValidationRules(issues)

	return recipe, nil
}

// MapIssuesToRecipes maps analysis issues to existing ARF recipes
func (a *ARFIntegration) MapIssuesToRecipes(issues []Issue) map[string][]string {
	recipeMap := make(map[string][]string)

	// Map common issue patterns to OpenRewrite recipes
	patternMap := map[string][]string{
		// Java patterns
		"NullPointer": {
			"org.openrewrite.java.cleanup.ExplicitInitialization",
			"org.openrewrite.java.cleanup.UseObjectNotifyAll",
		},
		"UnusedCode": {
			"org.openrewrite.java.cleanup.RemoveUnusedLocalVariables",
			"org.openrewrite.java.cleanup.RemoveUnusedPrivateMethods",
		},
		"Security": {
			"org.openrewrite.java.security.SecureRandom",
			"org.openrewrite.java.security.XmlParserXXEVulnerability",
		},
		"Performance": {
			"org.openrewrite.java.cleanup.UseStringReplace",
			"org.openrewrite.java.cleanup.ReplaceStringBuilderWithString",
		},
		// Python patterns
		"PythonSecurity": {
			"python.security.django-secure-settings",
			"python.security.avoid-eval",
		},
		// JavaScript patterns
		"JavaScriptSecurity": {
			"javascript.security.avoid-eval",
			"javascript.security.secure-cookies",
		},
	}

	for _, issue := range issues {
		for pattern, recipes := range patternMap {
			if a.matchesPattern(issue, pattern) {
				if _, exists := recipeMap[issue.ID]; !exists {
					recipeMap[issue.ID] = []string{}
				}
				recipeMap[issue.ID] = append(recipeMap[issue.ID], recipes...)
			}
		}
	}

	return recipeMap
}

// groupTriggersByRecipe groups ARF triggers by recipe name
func (a *ARFIntegration) groupTriggersByRecipe(triggers []ARFTrigger) map[string][]ARFTrigger {
	groups := make(map[string][]ARFTrigger)
	for _, trigger := range triggers {
		groups[trigger.RecipeName] = append(groups[trigger.RecipeName], trigger)
	}
	return groups
}

// processRecipeGroup processes a group of triggers for the same recipe
func (a *ARFIntegration) processRecipeGroup(ctx context.Context, repo Repository, recipeName string, triggers []ARFTrigger) error {
	// Create ARF repository structure
	arfRepo := arf.Repository{
		ID:       repo.ID,
		URL:      repo.URL,
		Branch:   repo.Branch,
		Language: repo.Language,
		Metadata: repo.Metadata,
	}

	// Create sandbox for transformation
	sandboxConfig := arf.SandboxConfig{
		Repository: arfRepo.URL,
		Branch:     arfRepo.Branch,
		TTL:        30 * time.Minute,
	}

	sandbox, err := a.sandboxManager.CreateSandbox(ctx, sandboxConfig)
	if err != nil {
		return fmt.Errorf("failed to create sandbox: %w", err)
	}
	defer func() {
		if derr := a.sandboxManager.DestroySandbox(ctx, sandbox.ID); derr != nil {
			a.logger.WithError(derr).Warn("failed to destroy sandbox")
		}
	}()

	// Execute recipe in sandbox
	// Note: This is a simplified integration - actual implementation would
	// use the full ARF transformation workflow
	a.logger.WithFields(logrus.Fields{
		"recipe":  recipeName,
		"sandbox": sandbox.ID,
		"issues":  len(triggers),
	}).Info("Executing recipe in sandbox")

	return nil
}

// filterCriticalIssues filters issues that require human review
func (a *ARFIntegration) filterCriticalIssues(issues []Issue) []Issue {
	critical := []Issue{}
	for _, issue := range issues {
		if issue.Severity == SeverityCritical ||
			(issue.Severity == SeverityHigh && issue.Category == CategorySecurity) {
			critical = append(critical, issue)
		}
	}
	return critical
}

// createWorkflowSteps removed - workflow functionality moved to transform command
/*
func (a *ARFIntegration) createWorkflowSteps(issues []Issue) []arf.WorkflowStep {
	// This functionality has been moved to the transform command's self-healing capabilities
	// Issues are now automatically remediated using LLM-powered solutions
				"issue_ids":    a.getIssueIDs(categoryIssues),
			},
		}
		steps = append(steps, step)
		stepID++
	}

	return steps
}
*/

// generateRecipeSteps generates recipe steps from issues
func (a *ARFIntegration) generateRecipeSteps(issues []Issue, language string) []*models.RecipeStep {
	steps := []*models.RecipeStep{}

	// Group similar issues
	issueGroups := a.groupSimilarIssues(issues)

	for groupName, groupIssues := range issueGroups {
		step := &models.RecipeStep{
			Name: fmt.Sprintf("Fix %s", groupName),
			Type: models.StepTypeRegexReplace,
			Config: map[string]interface{}{
				"pattern":     fmt.Sprintf("(%s issues pattern)", groupName),
				"replacement": "fixed version",
				"description": fmt.Sprintf("Remediate %d %s issues", len(groupIssues), groupName),
			},
		}
		steps = append(steps, step)
	}

	return steps
}

// generateValidationRules generates validation rules for issues
func (a *ARFIntegration) generateValidationRules(issues []Issue) models.ValidationRules {
	rules := models.ValidationRules{
		RequiredFiles: []string{},
		FilePatterns:  []string{},
	}

	// Add basic file requirements based on issues
	for _, issue := range issues {
		if issue.File != "" {
			// Add the file that has issues to required files for validation
			rules.RequiredFiles = append(rules.RequiredFiles, issue.File)
		}

		// Add patterns based on issue categories
		if issue.Category == "compilation" {
			rules.FilePatterns = append(rules.FilePatterns, "*.java", "*.go", "*.py")
		}
	}

	return rules
}

// Helper methods

func (a *ARFIntegration) calculateConfidence(issues []Issue) float64 {
	if len(issues) == 0 {
		return 1.0
	}

	// Calculate confidence based on issue severity distribution
	var totalWeight float64
	var confidenceSum float64

	for _, issue := range issues {
		weight := 1.0
		confidence := 0.5 // Base confidence

		switch issue.Severity {
		case SeverityCritical:
			weight = 3.0
			confidence = 0.3 // Lower confidence for critical issues
		case SeverityHigh:
			weight = 2.0
			confidence = 0.5
		case SeverityMedium:
			weight = 1.5
			confidence = 0.7
		case SeverityLow:
			weight = 1.0
			confidence = 0.8
		case SeverityInfo:
			weight = 0.5
			confidence = 0.9
		}

		totalWeight += weight
		confidenceSum += confidence * weight
	}

	if totalWeight == 0 {
		return 0.5
	}

	return confidenceSum / totalWeight
}

func (a *ARFIntegration) assessRiskLevel(issues []Issue) string {
	criticalCount := 0
	highCount := 0

	for _, issue := range issues {
		switch issue.Severity {
		case SeverityCritical:
			criticalCount++
		case SeverityHigh:
			highCount++
		}
	}

	if criticalCount > 0 {
		return "critical"
	} else if highCount > 2 {
		return "high"
	} else if highCount > 0 {
		return "medium"
	}

	return "low"
}

func (a *ARFIntegration) matchesPattern(issue Issue, pattern string) bool {
	// Simple pattern matching - can be enhanced with more sophisticated logic
	switch pattern {
	case "NullPointer":
		return issue.Category == CategoryBug &&
			(contains(issue.RuleName, "Null") || contains(issue.Message, "null"))
	case "UnusedCode":
		return contains(issue.RuleName, "Unused") || contains(issue.Message, "unused")
	case "Security":
		return issue.Category == CategorySecurity
	case "Performance":
		return issue.Category == CategoryPerformance
	default:
		return false
	}
}

func (a *ARFIntegration) groupSimilarIssues(issues []Issue) map[string][]Issue {
	groups := make(map[string][]Issue)

	for _, issue := range issues {
		key := fmt.Sprintf("%s-%s", issue.Category, issue.RuleName)
		groups[key] = append(groups[key], issue)
	}

	return groups
}

func (a *ARFIntegration) getIssueIDs(issues []Issue) []string {
	ids := make([]string, len(issues))
	for i, issue := range issues {
		ids[i] = issue.ID
	}
	return ids
}

// contains is a helper function for string containment
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				len(substr) < len(s) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

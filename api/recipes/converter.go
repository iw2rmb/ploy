package recipes

import (
	"regexp"
	"strings"
)

// RecipeConverter handles conversion of analysis results to OpenRewrite recipes
type RecipeConverter struct{}

// NewRecipeConverter creates a new recipe converter
func NewRecipeConverter() *RecipeConverter {
	return &RecipeConverter{}
}

// ConvertToOpenRewriteRecipe converts analysis to OpenRewrite recipe
func (rc *RecipeConverter) ConvertToOpenRewriteRecipe(analysis *LLMAnalysisResult, language string) (string, map[string]interface{}) {
	metadata := make(map[string]interface{})

	// Map common fixes to OpenRewrite recipes
	suggestedFix := strings.ToLower(analysis.SuggestedFix)

	switch language {
	case "java":
		if strings.Contains(suggestedFix, "add import") {
			// Extract import statement
			importPattern := regexp.MustCompile(`import\s+([\w\.]+);?`)
			if matches := importPattern.FindStringSubmatch(analysis.SuggestedFix); len(matches) > 1 {
				metadata["type"] = matches[1]
				metadata["onlyIfUsed"] = true
				return "org.openrewrite.java.AddImport", metadata
			}
		} else if strings.Contains(suggestedFix, "remove unused") {
			return "org.openrewrite.java.RemoveUnusedImports", metadata
		} else if analysis.ErrorType == "compilation" {
			return "org.openrewrite.java.cleanup.UnnecessaryThrows", metadata
		}

	case "python":
		if strings.Contains(suggestedFix, "remove unused import") {
			// Extract module name
			modulePattern := regexp.MustCompile(`import\s+(\w+)`)
			if matches := modulePattern.FindStringSubmatch(analysis.SuggestedFix); len(matches) > 1 {
				metadata["module"] = matches[1]
			}
			return "org.openrewrite.python.cleanup.RemoveUnusedImports", metadata
		}

	case "go":
		if strings.Contains(suggestedFix, "gofmt") || strings.Contains(suggestedFix, "format") {
			return "org.openrewrite.go.format", metadata
		}
	}

	// Default generic recipe
	return "org.openrewrite.text.Find", metadata
}

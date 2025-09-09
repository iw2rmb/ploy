package arf

import (
	"fmt"
	"strings"
)

// ParseOpenRewriteRecipeID parses an OpenRewrite recipe ID into its components
// This function maps recipe class names to appropriate Maven coordinates for OpenRewrite catalogs.
func ParseOpenRewriteRecipeID(recipeID string) (*OpenRewriteRecipeRequest, error) {
	// Validate that the recipe ID looks like a valid OpenRewrite recipe class
	if recipeID == "" {
		return nil, fmt.Errorf("recipe ID cannot be empty")
	}

	// All OpenRewrite recipes should use full class names (e.g., org.openrewrite.java.migrate.UpgradeToJava17)
	// Provide explicit Maven coordinates to ensure recipes are available in the runner without relying on discovery.
	// Mapping:
	//  - org.openrewrite.java.spring.*   → rewrite-spring
	//  - org.openrewrite.java.migrate.*  → rewrite-migrate-java
	//  - org.openrewrite.java.cleanup.*  → rewrite-java
	//  - default                         → rewrite-java
	artifact := "rewrite-java"
	switch {
	case strings.HasPrefix(recipeID, "org.openrewrite.java.spring"):
		artifact = "rewrite-spring"
	case strings.HasPrefix(recipeID, "org.openrewrite.java.migrate"):
		artifact = "rewrite-migrate-java"
	case strings.HasPrefix(recipeID, "org.openrewrite.java.cleanup"):
		artifact = "rewrite-java"
	default:
		artifact = "rewrite-java"
	}

	return &OpenRewriteRecipeRequest{
		RecipeClass:    recipeID,
		RecipeGroup:    "org.openrewrite.recipe",
		RecipeArtifact: artifact,
		RecipeVersion:  "2.20.0",
	}, nil
}

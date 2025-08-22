package arf

import (
	"context"
	"testing"
	"time"
)

func TestRecipeValidation(t *testing.T) {
	engine := &OpenRewriteEngine{}

	tests := []struct {
		name    string
		recipe  Recipe
		wantErr bool
	}{
		{
			name: "valid recipe",
			recipe: Recipe{
				ID:          "test-recipe",
				Name:        "Test Recipe",
				Description: "A test recipe",
				Language:    "java",
				Category:    CategoryCleanup,
				Confidence:  0.9,
				Source:      "org.openrewrite.java.cleanup.TestRecipe",
				Version:     "1.0.0",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			recipe: Recipe{
				Name:     "Test Recipe",
				Language: "java",
				Source:   "org.openrewrite.java.cleanup.TestRecipe",
			},
			wantErr: true,
		},
		{
			name: "missing source",
			recipe: Recipe{
				ID:       "test-recipe",
				Name:     "Test Recipe",
				Language: "java",
			},
			wantErr: true,
		},
		{
			name: "invalid source format",
			recipe: Recipe{
				ID:       "test-recipe",
				Name:     "Test Recipe",
				Language: "java",
				Source:   "InvalidClassName",
			},
			wantErr: true,
		},
		{
			name: "migration recipe missing version options",
			recipe: Recipe{
				ID:         "migration-recipe",
				Name:       "Migration Recipe",
				Language:   "java",
				Category:   CategoryMigration,
				Source:     "org.openrewrite.java.migrate.TestMigration",
				Options:    map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "security recipe low confidence",
			recipe: Recipe{
				ID:         "security-recipe",
				Name:       "Security Recipe",
				Language:   "java",
				Category:   CategorySecurity,
				Confidence: 0.5,
				Source:     "org.openrewrite.java.security.TestSecurity",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.ValidateRecipe(tt.recipe)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRecipe() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetRecipeMetadata(t *testing.T) {
	engine := &OpenRewriteEngine{
		recipes: make(map[string]Recipe),
	}

	// Add a test recipe
	recipe := Recipe{
		ID:          "test-recipe",
		Name:        "Test Recipe",
		Description: "A test recipe",
		Language:    "java",
		Category:    CategoryCleanup,
		Confidence:  0.9,
	}
	engine.recipes[recipe.ID] = recipe

	metadata, err := engine.GetRecipeMetadata("test-recipe")
	if err != nil {
		t.Fatalf("GetRecipeMetadata() error = %v", err)
	}

	if metadata.Recipe.ID != recipe.ID {
		t.Errorf("Expected recipe ID %s, got %s", recipe.ID, metadata.Recipe.ID)
	}

	if len(metadata.ApplicableLanguages) == 0 {
		t.Error("Expected applicable languages to be populated")
	}

	if metadata.SuccessRate != recipe.Confidence {
		t.Errorf("Expected success rate %f, got %f", recipe.Confidence, metadata.SuccessRate)
	}
}

func TestListAvailableRecipes(t *testing.T) {
	engine := &OpenRewriteEngine{
		recipes: make(map[string]Recipe),
	}

	recipes, err := engine.ListAvailableRecipes()
	if err != nil {
		t.Fatalf("ListAvailableRecipes() error = %v", err)
	}

	if len(recipes) == 0 {
		t.Error("Expected some default recipes to be available")
	}

	// Check that all recipes have required fields
	for _, recipe := range recipes {
		if recipe.ID == "" {
			t.Error("Recipe missing ID")
		}
		if recipe.Name == "" {
			t.Error("Recipe missing name")
		}
		if recipe.Language == "" {
			t.Error("Recipe missing language")
		}
		if recipe.Source == "" {
			t.Error("Recipe missing source")
		}
	}
}

func TestTransformationResult(t *testing.T) {
	result := &TransformationResult{
		RecipeID:        "test-recipe",
		Success:         true,
		ChangesApplied:  5,
		FilesModified:   []string{"Main.java", "Utils.java"},
		ExecutionTime:   2 * time.Second,
		ValidationScore: 0.95,
	}

	if !result.Success {
		t.Error("Expected transformation to be successful")
	}

	if result.ChangesApplied != 5 {
		t.Errorf("Expected 5 changes, got %d", result.ChangesApplied)
	}

	if len(result.FilesModified) != 2 {
		t.Errorf("Expected 2 modified files, got %d", len(result.FilesModified))
	}

	if result.ValidationScore < 0.9 {
		t.Errorf("Expected high validation score, got %f", result.ValidationScore)
	}
}

func TestCodebaseValidation(t *testing.T) {
	tests := []struct {
		name     string
		codebase Codebase
		valid    bool
	}{
		{
			name: "valid Java codebase",
			codebase: Codebase{
				Repository: "https://github.com/example/java-project",
				Branch:     "main",
				Language:   "java",
				BuildTool:  "maven",
			},
			valid: true,
		},
		{
			name: "valid Go codebase",
			codebase: Codebase{
				Repository: "https://github.com/example/go-project",
				Branch:     "main",
				Language:   "go",
				BuildTool:  "go",
			},
			valid: true,
		},
		{
			name: "codebase with metadata",
			codebase: Codebase{
				Repository: "https://github.com/example/project",
				Branch:     "develop",
				Language:   "python",
				BuildTool:  "pip",
				Metadata: map[string]string{
					"python_version": "3.9",
					"requirements":   "requirements.txt",
				},
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation checks
			if tt.codebase.Repository == "" && tt.valid {
				t.Error("Valid codebase should have repository")
			}
			if tt.codebase.Language == "" && tt.valid {
				t.Error("Valid codebase should have language")
			}
		})
	}
}

func TestRecipeCategories(t *testing.T) {
	categories := []RecipeCategory{
		CategoryCleanup,
		CategoryModernize,
		CategorySecurity,
		CategoryPerformance,
		CategoryMigration,
		CategoryStyle,
		CategoryTesting,
	}

	for _, category := range categories {
		if string(category) == "" {
			t.Errorf("Category should not be empty: %v", category)
		}
	}
}

func TestTransformationError(t *testing.T) {
	err := TransformationError{
		Type:        "parse_error",
		Message:     "Syntax error in file",
		File:        "Main.java",
		Line:        42,
		Column:      15,
		Recoverable: false,
	}

	if err.Type != "parse_error" {
		t.Errorf("Expected error type 'parse_error', got %s", err.Type)
	}

	if err.Line != 42 {
		t.Errorf("Expected line 42, got %d", err.Line)
	}

	if err.Recoverable {
		t.Error("Expected error to be non-recoverable")
	}
}
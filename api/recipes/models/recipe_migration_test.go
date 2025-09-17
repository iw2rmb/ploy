package models_test

import (
	"testing"

	recipesmodels "github.com/iw2rmb/ploy/api/recipes/models"
)

func TestRecipeGenerateIDAfterMigration(t *testing.T) {
	recipe := recipesmodels.Recipe{}
	recipe.Metadata.Name = "test-recipe"
	recipe.Metadata.Version = "1.2.3"

	if id := recipe.GenerateID(); id != "test-recipe-1.2.3" {
		t.Fatalf("expected id test-recipe-1.2.3, got %s", id)
	}
}

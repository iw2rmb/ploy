package recipes

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/api/recipes/models"
)

func TestRegistryStorageAdapterGetLatestRecipeReturnsNewestVersion(t *testing.T) {
	sea := &mockStorageProvider{}
	registry := NewRecipeRegistry(sea)
	adapter := NewRegistryStorageAdapter(registry)

	base := models.Recipe{
		Metadata: models.RecipeMetadata{
			Name:        "spring-upgrade",
			Description: "upgrade",
			Languages:   []string{"java"},
		},
		Steps: []models.RecipeStep{{
			Name:   "noop",
			Type:   models.StepTypeOpenRewrite,
			Config: map[string]any{"recipe": "org.sample"},
		}},
	}

	r10 := base
	r10.Metadata.Version = "1.0.0"
	r10.SetSystemFields("tester")
	if err := adapter.CreateRecipe(context.Background(), &r10); err != nil {
		t.Fatalf("seed 1.0.0: %v", err)
	}

	r120 := base
	r120.Metadata.Version = "1.2.0"
	r120.SetSystemFields("tester")
	if err := adapter.CreateRecipe(context.Background(), &r120); err != nil {
		t.Fatalf("seed 1.2.0: %v", err)
	}

	latest, err := adapter.GetLatestRecipe(context.Background(), "spring-upgrade")
	if err != nil {
		t.Fatalf("GetLatestRecipe: %v", err)
	}

	if latest.Metadata.Version != "1.2.0" {
		t.Fatalf("expected latest version 1.2.0, got %s", latest.Metadata.Version)
	}
}

package recipes

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// Use mockStorageProvider from registry_test.go (same package)

func TestStoreAndGetRecipe_AsModels(t *testing.T) {
	sea := &mockStorageProvider{}
	reg := NewRecipeRegistry(sea)
	// store
	r := &models.Recipe{ID: "r1", Metadata: models.RecipeMetadata{Name: "r1", Version: "1.0.0"}}
	if err := reg.StoreRecipe(context.Background(), r); err != nil {
		t.Fatalf("store error: %v", err)
	}
	// get back
	got, err := reg.GetRecipeAsModelsRecipe(context.Background(), "r1")
	if err != nil || got == nil {
		t.Fatalf("get error: %v", err)
	}
	if got.Metadata.Name != "r1" {
		t.Fatalf("unexpected name %s", got.Metadata.Name)
	}
}

package arf

import (
    "net/http/httptest"
    "testing"

    "github.com/gofiber/fiber/v2"
)

func setupRecipesApp(cat *RecipesCatalog) *fiber.App {
    app := fiber.New()
    h := NewRecipesHandler(cat)
    app.Get("/v1/arf/recipes", h.ListRecipes)
    app.Get("/v1/arf/recipes/:id", h.GetRecipe)
    return app
}

func TestRecipesHandlers_ListAndGet(t *testing.T) {
    // Seed catalog
    cat := NewRecipesCatalog()
    if err := cat.BuildFromYAMLs([][]byte{sampleRecipeYAML1, sampleRecipeYAML2}, "rewrite-java", "2.20.0"); err != nil {
        t.Fatalf("build catalog failed: %v", err)
    }

    app := setupRecipesApp(cat)

    // List endpoint
    req := httptest.NewRequest("GET", "/v1/arf/recipes?limit=5", nil)
    resp, err := app.Test(req, -1)
    if err != nil {
        t.Fatalf("list request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }

    // Get endpoint
    req2 := httptest.NewRequest("GET", "/v1/arf/recipes/org.openrewrite.java.RemoveUnusedImports", nil)
    resp2, err := app.Test(req2, -1)
    if err != nil {
        t.Fatalf("get request failed: %v", err)
    }
    if resp2.StatusCode != 200 {
        t.Fatalf("expected 200, got %d", resp2.StatusCode)
    }
}


package server

import (
    "net/http/httptest"
    "os"
    "testing"

    "github.com/gofiber/fiber/v2"
)

// Test that catalog routes can be registered behind a feature flag and respond.
func TestSetupRecipesCatalogRoutes_FeatureFlag(t *testing.T) {
    // Enable feature flag
    os.Setenv("PLOY_ENABLE_RECIPES_CATALOG", "true")
    defer os.Unsetenv("PLOY_ENABLE_RECIPES_CATALOG")

    s := &Server{app: fiber.New()}

    // Register only the catalog routes for this test
    s.setupRecipesCatalogRoutes()

    // Expect GET /v1/arf/recipes to respond 200 with JSON (array or object)
    req := httptest.NewRequest("GET", "/v1/arf/recipes", nil)
    resp, err := s.app.Test(req, -1)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }
}


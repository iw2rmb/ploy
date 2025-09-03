package server

import (
    "net/http/httptest"
    "os"
    "testing"

    "github.com/gofiber/fiber/v2"
)

// Recipes catalog routes should be always enabled; no feature flag required.
func TestSetupRecipesCatalogRoutes_AlwaysOn(t *testing.T) {
    // Ensure no environment flag is set
    os.Unsetenv("PLOY_ENABLE_RECIPES_CATALOG")

    s := &Server{app: fiber.New()}

    // Register the catalog routes
    s.setupRecipesCatalogRoutes()

    // Expect GET /v1/arf/recipes to respond 200 with JSON
    req := httptest.NewRequest("GET", "/v1/arf/recipes", nil)
    resp, err := s.app.Test(req, -1)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }
}

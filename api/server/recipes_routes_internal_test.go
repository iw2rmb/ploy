package server

import (
    "net/http/httptest"
    "strings"
    "testing"

    recipes "github.com/iw2rmb/ploy/internal/arf/recipes"
    providers_memory "github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

// Ensure GET /v1/arf/recipes/:id is wired to internal handler
func TestRecipesRoute_GetByID_Internal(t *testing.T) {
    t.Parallel()

    srv, err := NewServer(&ControllerConfig{})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }

    // Inject storage-backed registry with a minimal catalog
    mem := providers_memory.NewMemoryStorage(0)
    catalog := `[{"id":"org.openrewrite.java.cleanup.Cleanup","display_name":"Java Cleanup","tags":["cleanup"]}]`
    _ = mem.Put(nil, "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))
    srv.dependencies.ARFRecipes = recipes.NewStorageBacked(mem)

    req := httptest.NewRequest("GET", "/v1/arf/recipes/org.openrewrite.java.cleanup.Cleanup", nil)
    resp, err := srv.app.Test(req, -1)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }
}


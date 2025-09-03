package server

import (
    "encoding/json"
    "net/http/httptest"
    "strings"
    "testing"

    recipes "github.com/iw2rmb/ploy/internal/arf/recipes"
    providers_memory "github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

func TestARFRecipesPing_OK(t *testing.T) {
    t.Parallel()
    srv, err := NewServer(&ControllerConfig{})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }
    srv.app.Get("/v1/arf/recipes/ping", srv.handleARFRecipesPing)
    req := httptest.NewRequest("GET", "/v1/arf/recipes/ping", nil)
    resp, err := srv.app.Test(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }
}

func TestARFRecipesList_OK(t *testing.T) {
    t.Parallel()
    srv, err := NewServer(&ControllerConfig{})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }
    srv.app.Get("/v1/arf/recipes", srv.handleARFRecipesList)
    req := httptest.NewRequest("GET", "/v1/arf/recipes?language=java&tag=cleanup", nil)
    resp, err := srv.app.Test(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }
}

func TestARFRecipesList_StorageBacked_OK(t *testing.T) {
    t.Parallel()

    // Prepare a memory storage with a small catalog snapshot
    mem := providers_memory.NewMemoryStorage(0)
    catalog := `[
      {"id":"org.openrewrite.java.cleanup.Cleanup","display_name":"Java Cleanup","description":"Cleanup rules","tags":["cleanup","java"],"pack":"rewrite-java","version":"1.2.3"},
      {"id":"org.openrewrite.java.format.AutoFormat","display_name":"Auto Format","description":"Formatting","tags":["format","java"],"pack":"rewrite-java","version":"1.0.0"}
    ]`
    // write to the expected catalog key
    _ = mem.Put(nil, "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

    // Build a server and inject storage-backed registry
    srv, err := NewServer(&ControllerConfig{})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }
    // Overwrite ARFRecipes with storage-backed registry
    srv.dependencies.ARFRecipes = recipes.NewStorageBacked(mem)

    srv.app.Get("/v1/arf/recipes", srv.handleARFRecipesList)
    req := httptest.NewRequest("GET", "/v1/arf/recipes?tag=cleanup", nil)
    resp, err := srv.app.Test(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }
}

func TestARFRecipesList_StorageBacked_LanguageFilter(t *testing.T) {
    t.Parallel()

    mem := providers_memory.NewMemoryStorage(0)
    catalog := `[
      {"id":"org.openrewrite.java.cleanup.Cleanup","display_name":"Java Cleanup","tags":["cleanup","java"]},
      {"id":"org.openrewrite.kotlin.cleanup.Cleanup","display_name":"Kotlin Cleanup","tags":["cleanup","kotlin"]}
    ]`
    _ = mem.Put(nil, "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

    srv, err := NewServer(&ControllerConfig{})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }
    srv.dependencies.ARFRecipes = recipes.NewStorageBacked(mem)

    srv.app.Get("/v1/arf/recipes", srv.handleARFRecipesList)
    req := httptest.NewRequest("GET", "/v1/arf/recipes?language=java", nil)
    resp, err := srv.app.Test(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }
    var body map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
        t.Fatalf("decode: %v", err)
    }
    arr, _ := body["recipes"].([]interface{})
    if len(arr) != 1 {
        t.Fatalf("expected 1 recipe for language=java, got %d", len(arr))
    }
}

func TestARFRecipesGet_StorageBacked_OK(t *testing.T) {
    t.Parallel()

    mem := providers_memory.NewMemoryStorage(0)
    catalog := `[
      {"id":"org.openrewrite.java.cleanup.Cleanup","display_name":"Java Cleanup","description":"Cleanup rules","tags":["cleanup","java"],"pack":"rewrite-java","version":"1.2.3"},
      {"id":"org.openrewrite.java.format.AutoFormat","display_name":"Auto Format","description":"Formatting","tags":["format","java"],"pack":"rewrite-java","version":"1.0.0"}
    ]`
    _ = mem.Put(nil, "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

    srv, err := NewServer(&ControllerConfig{})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }
    srv.dependencies.ARFRecipes = recipes.NewStorageBacked(mem)

    // Register our internal handler on a test path to avoid overlay
    srv.app.Get("/v1/arf/recipes/_test/:id", srv.handleARFRecipesGet)

    req := httptest.NewRequest("GET", "/v1/arf/recipes/_test/org.openrewrite.java.cleanup.Cleanup", nil)
    resp, err := srv.app.Test(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }
}

func TestARFRecipesList_StorageBacked_PayloadFields(t *testing.T) {
    t.Parallel()

    mem := providers_memory.NewMemoryStorage(0)
    catalog := `[
      {"id":"org.openrewrite.java.format.AutoFormat","display_name":"Auto Format","description":"Formatting rules","tags":["format","java"],"pack":"rewrite-java","version":"1.0.0"}
    ]`
    _ = mem.Put(nil, "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

    srv, err := NewServer(&ControllerConfig{})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }
    srv.dependencies.ARFRecipes = recipes.NewStorageBacked(mem)

    srv.app.Get("/v1/arf/recipes", srv.handleARFRecipesList)
    req := httptest.NewRequest("GET", "/v1/arf/recipes", nil)
    resp, err := srv.app.Test(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }
    var body map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
        t.Fatalf("decode: %v", err)
    }
    arr, _ := body["recipes"].([]interface{})
    if len(arr) != 1 {
        t.Fatalf("expected 1 recipe, got %d", len(arr))
    }
    rec := arr[0].(map[string]interface{})
    if rec["description"] == nil || rec["pack"] == nil || rec["version"] == nil {
        t.Fatalf("expected description, pack, version present in list payload")
    }
}

func TestARFRecipesGet_StorageBacked_PayloadFields(t *testing.T) {
    t.Parallel()

    mem := providers_memory.NewMemoryStorage(0)
    catalog := `[
      {"id":"org.openrewrite.java.cleanup.Cleanup","display_name":"Java Cleanup","description":"Cleanup rules","tags":["cleanup","java"],"pack":"rewrite-java","version":"1.2.3"}
    ]`
    _ = mem.Put(nil, "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

    srv, err := NewServer(&ControllerConfig{})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }
    srv.dependencies.ARFRecipes = recipes.NewStorageBacked(mem)

    req := httptest.NewRequest("GET", "/v1/arf/recipes/org.openrewrite.java.cleanup.Cleanup", nil)
    resp, err := srv.app.Test(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }
    var body map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
        t.Fatalf("decode: %v", err)
    }
    rec := body["recipe"].(map[string]interface{})
    if rec["description"] == nil || rec["pack"] == nil || rec["version"] == nil {
        t.Fatalf("expected description, pack, version present in get payload")
    }
}

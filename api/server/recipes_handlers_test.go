package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	recipes "github.com/iw2rmb/ploy/internal/arf/recipes"
	providers_memory "github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

func TestRecipeCatalogPing_OK(t *testing.T) {
	t.Parallel()

	srv, err := NewServer(&ControllerConfig{})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	srv.app.Get("/_test/recipes/ping", srv.handleRecipeCatalogPing)
	req := httptest.NewRequest("GET", "/_test/recipes/ping", nil)
	resp := mustResponse(t)(srv.app.Test(req))
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestRecipeCatalogList_OK(t *testing.T) {
	t.Parallel()
	srv, err := NewServer(&ControllerConfig{})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	srv.app.Get("/_test/recipes", srv.handleRecipeCatalogList)
	req := httptest.NewRequest("GET", "/_test/recipes?language=java&tag=cleanup", nil)
	resp := mustResponse(t)(srv.app.Test(req))
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestRecipeCatalogList_StorageBacked_OK(t *testing.T) {
	t.Parallel()

	mem := providers_memory.NewMemoryStorage(0)
	catalog := `[
      {"id":"org.openrewrite.java.cleanup.Cleanup","display_name":"Java Cleanup","description":"Cleanup rules","tags":["cleanup","java"],"pack":"rewrite-java","version":"1.2.3"},
      {"id":"org.openrewrite.java.format.AutoFormat","display_name":"Auto Format","description":"Formatting","tags":["format","java"],"pack":"rewrite-java","version":"1.0.0"}
    ]`
	_ = mem.Put(context.TODO(), "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

	srv, err := NewServer(&ControllerConfig{})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	srv.dependencies.RecipeCatalog = recipes.NewStorageBacked(mem)

	srv.app.Get("/_test/recipes", srv.handleRecipeCatalogList)
	req := httptest.NewRequest("GET", "/_test/recipes?tag=cleanup", nil)
	resp := mustResponse(t)(srv.app.Test(req))
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestRecipeCatalogList_StorageBacked_LanguageFilter(t *testing.T) {
	t.Parallel()

	mem := providers_memory.NewMemoryStorage(0)
	catalog := `[
      {"id":"org.openrewrite.java.cleanup.Cleanup","display_name":"Java Cleanup","tags":["cleanup","java"]},
      {"id":"org.openrewrite.kotlin.cleanup.Cleanup","display_name":"Kotlin Cleanup","tags":["cleanup","kotlin"]}
    ]`
	_ = mem.Put(context.TODO(), "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

	srv, err := NewServer(&ControllerConfig{})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	srv.dependencies.RecipeCatalog = recipes.NewStorageBacked(mem)

	srv.app.Get("/_test/recipes", srv.handleRecipeCatalogList)
	req := httptest.NewRequest("GET", "/_test/recipes?language=java", nil)
	resp := mustResponse(t)(srv.app.Test(req))
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

func TestRecipeCatalogGet_StorageBacked_OK(t *testing.T) {
	t.Parallel()

	mem := providers_memory.NewMemoryStorage(0)
	catalog := `[
      {"id":"org.openrewrite.java.cleanup.Cleanup","display_name":"Java Cleanup","description":"Cleanup rules","tags":["cleanup","java"],"pack":"rewrite-java","version":"1.2.3"},
      {"id":"org.openrewrite.java.format.AutoFormat","display_name":"Auto Format","description":"Formatting","tags":["format","java"],"pack":"rewrite-java","version":"1.0.0"}
    ]`
	_ = mem.Put(context.TODO(), "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

	srv, err := NewServer(&ControllerConfig{})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	srv.dependencies.RecipeCatalog = recipes.NewStorageBacked(mem)

	srv.app.Get("/_test/recipes/:id", srv.handleRecipeCatalogGet)

	req := httptest.NewRequest("GET", "/_test/recipes/org.openrewrite.java.cleanup.Cleanup", nil)
	resp := mustResponse(t)(srv.app.Test(req))
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestRecipeCatalogList_StorageBacked_PayloadFields(t *testing.T) {
	t.Parallel()

	mem := providers_memory.NewMemoryStorage(0)
	catalog := `[
      {"id":"org.openrewrite.java.format.AutoFormat","display_name":"Auto Format","description":"Formatting rules","tags":["format","java"],"pack":"rewrite-java","version":"1.0.0"}
    ]`
	_ = mem.Put(context.TODO(), "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

	srv, err := NewServer(&ControllerConfig{})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	srv.dependencies.RecipeCatalog = recipes.NewStorageBacked(mem)

	srv.app.Get("/_test/recipes", srv.handleRecipeCatalogList)
	req := httptest.NewRequest("GET", "/_test/recipes", nil)
	resp := mustResponse(t)(srv.app.Test(req))
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestRecipeCatalogGet_StorageBacked_PayloadFields(t *testing.T) {
	t.Parallel()

	mem := providers_memory.NewMemoryStorage(0)
	catalog := `[
      {"id":"org.openrewrite.java.format.AutoFormat","display_name":"Auto Format","description":"Formatting rules","tags":["format","java"],"pack":"rewrite-java","version":"1.0.0"}
    ]`
	_ = mem.Put(context.TODO(), "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

	srv, err := NewServer(&ControllerConfig{})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	srv.dependencies.RecipeCatalog = recipes.NewStorageBacked(mem)

	srv.app.Get("/_test/recipes/:id", srv.handleRecipeCatalogGet)

	req := httptest.NewRequest("GET", "/_test/recipes/org.openrewrite.java.format.AutoFormat", nil)
	resp := mustResponse(t)(srv.app.Test(req))
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

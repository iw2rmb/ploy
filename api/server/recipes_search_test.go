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

func TestRecipeCatalogSearch_Query_OK(t *testing.T) {
	t.Parallel()

	mem := providers_memory.NewMemoryStorage(0)
	catalog := `[
      {"id":"org.openrewrite.java.format.AutoFormat","display_name":"Auto Format","description":"Formatting rules","tags":["format","java"]},
      {"id":"org.openrewrite.java.cleanup.Cleanup","display_name":"Java Cleanup","description":"Cleanup rules","tags":["cleanup","java"]}
    ]`
	_ = mem.Put(context.TODO(), "artifacts/openrewrite/catalog.json", strings.NewReader(catalog))

	srv, err := NewServer(&ControllerConfig{})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	srv.dependencies.RecipeCatalog = recipes.NewStorageBacked(mem)

	// wire search under test-specific path to avoid clashes with production routes
	srv.app.Get("/_test/recipes/search", srv.handleRecipeCatalogSearch)

	req := httptest.NewRequest("GET", "/_test/recipes/search?q=Format", nil)
	resp := mustResponse(t)(srv.app.Test(req))
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	arr, _ := body["recipes"].([]interface{})
	if len(arr) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(arr))
	}
}

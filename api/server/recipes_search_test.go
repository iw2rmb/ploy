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

func TestARFRecipesSearch_Query_OK(t *testing.T) {
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
	srv.dependencies.ARFRecipes = recipes.NewStorageBacked(mem)

	// wire search under /v1/arf/recipes/search for test
	srv.app.Get("/v1/arf/recipes/search", srv.handleARFRecipesSearch)

	req := httptest.NewRequest("GET", "/v1/arf/recipes/search?q=Format", nil)
	resp, err := srv.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
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

package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	recipes "github.com/iw2rmb/ploy/internal/recipes/catalog"
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
	resp1, err := srv.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp1 != nil && resp1.Body != nil {
			_ = resp1.Body.Close()
		}
	})
	if resp1.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp1.StatusCode)
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
	resp2, err := srv.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp2 != nil && resp2.Body != nil {
			_ = resp2.Body.Close()
		}
	})
	if resp2.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp2.StatusCode)
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
	resp3, err := srv.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp3 != nil && resp3.Body != nil {
			_ = resp3.Body.Close()
		}
	})
	if resp3.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp3.StatusCode)
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
	resp4, err := srv.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp4 != nil && resp4.Body != nil {
			_ = resp4.Body.Close()
		}
	})
	if resp4.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp4.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp4.Body).Decode(&body); err != nil {
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
	resp5, err := srv.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp5 != nil && resp5.Body != nil {
			_ = resp5.Body.Close()
		}
	})
	if resp5.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp5.StatusCode)
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
	resp6, err := srv.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp6 != nil && resp6.Body != nil {
			_ = resp6.Body.Close()
		}
	})
	if resp6.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp6.StatusCode)
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
	resp7, err := srv.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp7 != nil && resp7.Body != nil {
			_ = resp7.Body.Close()
		}
	})
	if resp7.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp7.StatusCode)
	}
}

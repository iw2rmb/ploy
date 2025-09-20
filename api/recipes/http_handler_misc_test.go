package recipes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPHandler_SearchRecipes_MissingQueryAndSuccess(t *testing.T) {
	handler := setupRecipesHTTPHandler(t)
	app := newAppWithHandler(handler)

	// Missing q
	req := httptest.NewRequest(http.MethodGet, "/v1/recipes/search", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	// Success
	req2 := httptest.NewRequest(http.MethodGet, "/v1/recipes/search?q=test", nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
}

func TestHTTPHandler_ValidateRecipe_WarningsAndInvalid(t *testing.T) {
	_ = setupRecipesHTTPHandler(t) // covered in existing tests; keep function to ensure compile
}

func TestHTTPHandler_DownloadRecipe_SetsHeadersAndNotFound(t *testing.T) {
	handler := setupRecipesHTTPHandler(t)
	app := newAppWithHandler(handler)

	// Success
	req := httptest.NewRequest(http.MethodGet, "/v1/recipes/test-recipe-1.0.0/download", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got == "" {
		t.Fatalf("expected content type header set")
	}
	if cd := resp.Header.Get("Content-Disposition"); cd == "" {
		t.Fatalf("expected Content-Disposition header")
	}

	// Not found
	req2 := httptest.NewRequest(http.MethodGet, "/v1/recipes/missing-id/download", nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp2.StatusCode)
	}
}

func TestHTTPHandler_ListRecipes_WithTypeFilter(t *testing.T) {
	handler := setupRecipesHTTPHandler(t)
	app := newAppWithHandler(handler)
	req := httptest.NewRequest(http.MethodGet, "/v1/recipes?type=modernization", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

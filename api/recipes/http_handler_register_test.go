package recipes

import (
    "bytes"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestHTTPHandler_RegisterRecipeFromRunner_Success(t *testing.T) {
    handler := setupRecipesHTTPHandler(t)
    app := newAppWithHandler(handler)

    body := []byte(`{"recipe_class":"org.example.MyRecipe","maven_coords":"com.example:my-recipe:1.0.0","jar_path":"/tmp/app.jar"}`)
    req := httptest.NewRequest(http.MethodPost, "/v1/recipes/register", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != 200 { t.Fatalf("expected 200, got %d", resp.StatusCode) }
}

func TestHTTPHandler_RegisterRecipeFromRunner_MissingFields(t *testing.T) {
    handler := setupRecipesHTTPHandler(t)
    app := newAppWithHandler(handler)
    body := []byte(`{"recipe_class":""}`)
    req := httptest.NewRequest(http.MethodPost, "/v1/recipes/register", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusBadRequest { t.Fatalf("expected 400, got %d", resp.StatusCode) }
}

func TestHTTPHandler_RegisterRecipeFromRunner_RegistryUnavailable(t *testing.T) {
    // Construct handler without provider/registry
    h := NewHTTPHandlerWithStorage(nil, nil, nil, nil, nil)
    app := newAppWithHandler(h)
    body := []byte(`{"recipe_class":"org.example.Recipe","maven_coords":"g:a:1.0.0"}`)
    req := httptest.NewRequest(http.MethodPost, "/v1/recipes/register", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusServiceUnavailable { t.Fatalf("expected 503, got %d", resp.StatusCode) }
}


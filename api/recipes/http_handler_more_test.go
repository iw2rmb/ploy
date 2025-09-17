package recipes

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/iw2rmb/ploy/api/recipes/models"
    "github.com/iw2rmb/ploy/internal/storage"
)

func newAppWithHandler(h *HTTPHandler) *fiber.App {
    app := fiber.New()
    h.RegisterRoutes(app)
    return app
}

// deletableProvider implements minimal StorageProvider with Delete support for tests
type deletableProvider struct{ data map[string][]byte }

func newDeletableProvider() *deletableProvider { return &deletableProvider{data: map[string][]byte{}} }

func (p *deletableProvider) PutObject(_ string, key string, body io.ReadSeeker, _ string) (*storage.PutObjectResult, error) {
    b, _ := io.ReadAll(body)
    p.data[key] = b
    return &storage.PutObjectResult{Location: key, Size: int64(len(b))}, nil
}
func (p *deletableProvider) UploadArtifactBundle(keyPrefix, artifactPath string) error { return nil }
func (p *deletableProvider) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*storage.BundleIntegrityResult, error) { return &storage.BundleIntegrityResult{Verified: true}, nil }
func (p *deletableProvider) GetObject(_ string, key string) (io.ReadCloser, error) {
    if b, ok := p.data[key]; ok { return io.NopCloser(bytes.NewReader(b)), nil }
    return nil, io.EOF
}
func (p *deletableProvider) VerifyUpload(string) error { return nil }
func (p *deletableProvider) ListObjects(_ string, prefix string) ([]storage.ObjectInfo, error) {
    out := make([]storage.ObjectInfo, 0, len(p.data))
    for k, v := range p.data { if prefix == "" || (len(k) >= len(prefix) && k[:len(prefix)] == prefix) { out = append(out, storage.ObjectInfo{Key:k, Size:int64(len(v))}) } }
    return out, nil
}
func (p *deletableProvider) GetProviderType() string    { return "seaweedfs" }
func (p *deletableProvider) GetArtifactsBucket() string { return "ploy-recipes" }
func (p *deletableProvider) Delete(_ context.Context, key string) error { delete(p.data, key); return nil }

func setupRecipesHTTPHandlerWithDeletable(t *testing.T) *HTTPHandler {
    t.Helper()
    prov := newDeletableProvider()
    registry := NewRecipeRegistry(prov)
    storageAdapter := NewRegistryStorageAdapter(registry)
    return NewHTTPHandlerWithStorage(storageAdapter, nil, nil, prov, registry)
}

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

func TestHTTPHandler_GetRecipe_NotFound(t *testing.T) {
    handler := setupRecipesHTTPHandler(t)
    app := newAppWithHandler(handler)
    req := httptest.NewRequest(http.MethodGet, "/v1/recipes/does-not-exist", nil)
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusNotFound { t.Fatalf("expected 404, got %d", resp.StatusCode) }
}

func TestHTTPHandler_GetRecipe_RegistryUnavailable(t *testing.T) {
    h := NewHTTPHandlerWithStorage(nil, nil, nil, nil, nil)
    app := newAppWithHandler(h)
    req := httptest.NewRequest(http.MethodGet, "/v1/recipes/any", nil)
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusServiceUnavailable { t.Fatalf("expected 503, got %d", resp.StatusCode) }
}

func TestHTTPHandler_UpdateRecipe_Success(t *testing.T) {
    handler := setupRecipesHTTPHandler(t)
    app := newAppWithHandler(handler)
    // Seeded by setup: test-recipe-1.0.0
    update := &models.Recipe{
        Metadata: models.RecipeMetadata{
            Name:        "test-recipe",
            Description: "updated",
            Version:     "1.0.0",
            Languages:   []string{"java"},
            Author:      "tester",
        },
        Steps: []models.RecipeStep{{
            Name:    "cleanup",
            Type:    models.StepTypeOpenRewrite,
            Config:  map[string]any{"recipe": "org.openrewrite.java.cleanup.Sample"},
            Timeout: models.Duration{Duration: time.Minute},
        }},
    }
    b, _ := json.Marshal(update)
    req := httptest.NewRequest(http.MethodPut, "/v1/recipes/test-recipe-1.0.0", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp.StatusCode) }
}

func TestHTTPHandler_UpdateRecipe_InvalidJSON(t *testing.T) {
    handler := setupRecipesHTTPHandler(t)
    app := newAppWithHandler(handler)
    req := httptest.NewRequest(http.MethodPut, "/v1/recipes/test-recipe-1.0.0", bytes.NewReader([]byte("{")))
    req.Header.Set("Content-Type", "application/json")
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusBadRequest { t.Fatalf("expected 400, got %d", resp.StatusCode) }
}

func TestHTTPHandler_DeleteRecipe_Success(t *testing.T) {
    handler := setupRecipesHTTPHandlerWithDeletable(t)
    store := handler.storage.(*RegistryStorageAdapter)
    r := &models.Recipe{
        Metadata: models.RecipeMetadata{
            Name:        "del-recipe",
            Description: "desc",
            Version:     "1.0.0",
            Author:      "tester",
            Languages:   []string{"java"},
        },
        Steps: []models.RecipeStep{{Name: "s", Type: models.StepTypeOpenRewrite, Config: map[string]any{"recipe":"x"}}},
    }
    r.SetSystemFields("tester")
    if err := store.CreateRecipe(context.Background(), r); err != nil { t.Fatalf("seed: %v", err) }

    app := newAppWithHandler(handler)

    // First delete existing
    req := httptest.NewRequest(http.MethodDelete, "/v1/recipes/del-recipe-1.0.0", nil)
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp.StatusCode) }

    // A subsequent delete may be treated as no-op by provider; no further assertion
}

func TestHTTPHandler_SearchRecipes_MissingQueryAndSuccess(t *testing.T) {
    handler := setupRecipesHTTPHandler(t)
    app := newAppWithHandler(handler)

    // Missing q
    req := httptest.NewRequest(http.MethodGet, "/v1/recipes/search", nil)
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusBadRequest { t.Fatalf("expected 400, got %d", resp.StatusCode) }

    // Success
    req2 := httptest.NewRequest(http.MethodGet, "/v1/recipes/search?q=test", nil)
    resp2, err := app.Test(req2)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp2.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp2.StatusCode) }
}

func TestHTTPHandler_ValidateRecipe_WarningsAndInvalid(t *testing.T) {
    handler := setupRecipesHTTPHandler(t)
    app := newAppWithHandler(handler)

    // Valid recipe with warnings (no license, maybe missing timeouts)
    r := &models.Recipe{
        Metadata: models.RecipeMetadata{
            Name:        "warn-recipe",
            Description: "desc",
            Version:     "1.0.0",
            Author:      "tester",
            Languages:   []string{"java"},
        },
        Steps: []models.RecipeStep{{
            Name:   "cleanup",
            Type:   models.StepTypeOpenRewrite,
            Config: map[string]any{"recipe": "org.openrewrite.java.cleanup.Sample"},
        }},
    }
    b, _ := json.Marshal(r)
    req := httptest.NewRequest(http.MethodPost, "/v1/recipes/validate", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp.StatusCode) }

    // Invalid recipe (missing name)
    r.Metadata.Name = ""
    b2, _ := json.Marshal(r)
    req2 := httptest.NewRequest(http.MethodPost, "/v1/recipes/validate", bytes.NewReader(b2))
    req2.Header.Set("Content-Type", "application/json")
    resp2, err := app.Test(req2)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp2.StatusCode != http.StatusBadRequest { t.Fatalf("expected 400, got %d", resp2.StatusCode) }
}

func TestHTTPHandler_DownloadRecipe_SetsHeadersAndNotFound(t *testing.T) {
    handler := setupRecipesHTTPHandler(t)
    app := newAppWithHandler(handler)

    // Success
    req := httptest.NewRequest(http.MethodGet, "/v1/recipes/test-recipe-1.0.0/download", nil)
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp.StatusCode) }
    if got := resp.Header.Get("Content-Type"); got == "" {
        t.Fatalf("expected content type header set")
    }
    if cd := resp.Header.Get("Content-Disposition"); cd == "" {
        t.Fatalf("expected Content-Disposition header")
    }

    // Not found
    req2 := httptest.NewRequest(http.MethodGet, "/v1/recipes/missing-id/download", nil)
    resp2, err := app.Test(req2)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp2.StatusCode != http.StatusNotFound { t.Fatalf("expected 404, got %d", resp2.StatusCode) }
}

func TestHTTPHandler_ListRecipes_WithTypeFilter(t *testing.T) {
    // Build handler and store a second recipe with differing metadata
    handler := setupRecipesHTTPHandler(t)
    store := handler.storage.(*RegistryStorageAdapter)
    r2 := &models.Recipe{
        Metadata: models.RecipeMetadata{
            Name:        "java-modernize",
            Description: "desc",
            Version:     "1.0.0",
            Author:      "tester",
            Languages:   []string{"java"},
            Categories:  []string{"modernization"},
            Tags:        []string{"type:refactoring"},
        },
        Steps: []models.RecipeStep{{
            Name:   "cleanup2",
            Type:   models.StepTypeOpenRewrite,
            Config: map[string]any{"recipe": "org.openrewrite.java.cleanup.Another"},
        }},
    }
    r2.SetSystemFields("tester")
    if err := store.CreateRecipe(context.Background(), r2); err != nil {
        t.Fatalf("seed second recipe: %v", err)
    }

    app := newAppWithHandler(handler)
    req := httptest.NewRequest(http.MethodGet, "/v1/recipes?type=modernization", nil)
    resp, err := app.Test(req)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp.StatusCode) }
}

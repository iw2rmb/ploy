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

type testSeaweed struct{ data map[string][]byte }

func newTestSeaweed() *testSeaweed { return &testSeaweed{data: map[string][]byte{}} }

func (m *testSeaweed) PutObject(_ string, key string, body io.ReadSeeker, _ string) (*storage.PutObjectResult, error) {
	b, _ := io.ReadAll(body)
	m.data[key] = b
	return &storage.PutObjectResult{Location: key, Size: int64(len(b))}, nil
}

func (m *testSeaweed) UploadArtifactBundle(string, string) error { return nil }
func (m *testSeaweed) UploadArtifactBundleWithVerification(string, string) (*storage.BundleIntegrityResult, error) {
	return &storage.BundleIntegrityResult{Verified: true}, nil
}
func (m *testSeaweed) VerifyUpload(string) error { return nil }

func (m *testSeaweed) GetObject(_ string, key string) (io.ReadCloser, error) {
	if b, ok := m.data[key]; ok {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	return nil, io.EOF
}

func (m *testSeaweed) ListObjects(_ string, prefix string) ([]storage.ObjectInfo, error) {
	out := make([]storage.ObjectInfo, 0, len(m.data))
	for k, v := range m.data {
		if len(prefix) == 0 || len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, storage.ObjectInfo{Key: k, Size: int64(len(v)), ContentType: "application/x-yaml"})
		}
	}
	return out, nil
}

func (m *testSeaweed) GetProviderType() string    { return "seaweedfs" }
func (m *testSeaweed) GetArtifactsBucket() string { return "ploy-recipes" }

func setupRecipesHTTPHandler(t *testing.T) *HTTPHandler {
	t.Helper()

	store := newTestSeaweed()
	registry := NewRecipeRegistry(store)
	storageAdapter := NewRegistryStorageAdapter(registry)
	handler := NewHTTPHandlerWithStorage(storageAdapter, nil, nil, store, registry)

	recipe := &models.Recipe{
		Metadata: models.RecipeMetadata{
			Name:        "test-recipe",
			Description: "test",
			Version:     "1.0.0",
			Languages:   []string{"java"},
			Author:      "tester",
		},
		Steps: []models.RecipeStep{{
			Name:   "cleanup",
			Type:   models.StepTypeOpenRewrite,
			Config: map[string]interface{}{"recipe": "org.openrewrite.java.cleanup.Sample"},
		}},
	}
	recipe.SetSystemFields("tester")
	if err := storageAdapter.CreateRecipe(context.Background(), recipe); err != nil {
		t.Fatalf("seed recipe: %v", err)
	}

	return handler
}

func TestHTTPHandlerListRecipes(t *testing.T) {
	handler := setupRecipesHTTPHandler(t)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest(http.MethodGet, "/v1/recipes", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHTTPHandlerCreateRecipe(t *testing.T) {
	handler := setupRecipesHTTPHandler(t)
	app := fiber.New()
	handler.RegisterRoutes(app)

	recipe := &models.Recipe{
		Metadata: models.RecipeMetadata{
			Name:        "sample",
			Description: "sample",
			Version:     "1.0.0",
			Languages:   []string{"java"},
		},
		Steps: []models.RecipeStep{{
			Name:    "cleanup",
			Type:    models.StepTypeOpenRewrite,
			Config:  map[string]interface{}{"recipe": "org.openrewrite.java.cleanup.Sample"},
			Timeout: models.Duration{Duration: time.Minute},
		}},
	}

	body, _ := json.Marshal(recipe)
	req := httptest.NewRequest(http.MethodPost, "/v1/recipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}
}

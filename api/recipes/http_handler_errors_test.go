package recipes

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/recipes/models"
	"github.com/iw2rmb/ploy/internal/storage"
)

type failingStorageProvider struct {
	data    map[string][]byte
	putErr  error
	listErr error
}

func newFailingStorageProvider(opts ...func(*failingStorageProvider)) *failingStorageProvider {
	f := &failingStorageProvider{data: make(map[string][]byte)}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

func withPutError(err error) func(*failingStorageProvider) {
	return func(f *failingStorageProvider) {
		f.putErr = err
	}
}

func withListError(err error) func(*failingStorageProvider) {
	return func(f *failingStorageProvider) {
		f.listErr = err
	}
}

func (f *failingStorageProvider) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*storage.PutObjectResult, error) {
	if f.putErr != nil {
		return nil, f.putErr
	}
	b, _ := io.ReadAll(body)
	f.data[bucket+"/"+key] = b
	return &storage.PutObjectResult{Location: key, Size: int64(len(b))}, nil
}

func (f *failingStorageProvider) UploadArtifactBundle(string, string) error { return nil }
func (f *failingStorageProvider) UploadArtifactBundleWithVerification(string, string) (*storage.BundleIntegrityResult, error) {
	return &storage.BundleIntegrityResult{Verified: true}, nil
}
func (f *failingStorageProvider) VerifyUpload(string) error { return nil }

func (f *failingStorageProvider) GetObject(bucket, key string) (io.ReadCloser, error) {
	if data, ok := f.data[bucket+"/"+key]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return nil, errors.New("not found")
}

func (f *failingStorageProvider) ListObjects(bucket, prefix string) ([]storage.ObjectInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	var infos []storage.ObjectInfo
	for key := range f.data {
		if strings.HasPrefix(key, bucket+"/"+prefix) {
			infos = append(infos, storage.ObjectInfo{Key: strings.TrimPrefix(key, bucket+"/")})
		}
	}
	return infos, nil
}

func (f *failingStorageProvider) GetProviderType() string    { return "failing" }
func (f *failingStorageProvider) GetArtifactsBucket() string { return "ploy-recipes" }

func setupHandlerWithStorageProvider(t *testing.T, provider storage.StorageProvider) *fiber.App {
	t.Helper()
	registry := NewRecipeRegistry(provider)
	adapter := NewRegistryStorageAdapter(registry)
	handler := NewHTTPHandlerWithStorage(adapter, nil, nil, nil, registry)

	app := fiber.New()
	handler.RegisterRoutes(app)
	return app
}

func TestHTTPHandlerCreateRecipeInvalidJSON(t *testing.T) {
	app := setupHandlerWithStorageProvider(t, newTestSeaweed())

	req := httptest.NewRequest(http.MethodPost, "/v1/recipes", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestHTTPHandlerCreateRecipeStorageFailure(t *testing.T) {
	provider := newFailingStorageProvider(withPutError(errors.New("put failed")))
	app := setupHandlerWithStorageProvider(t, provider)

	recipe := &models.Recipe{
		Metadata: models.RecipeMetadata{
			Name:        "fail",
			Description: "should fail",
			Version:     "1.0.0",
			Languages:   []string{"java"},
		},
		Steps: []models.RecipeStep{{
			Name:   "noop",
			Type:   models.StepTypeOpenRewrite,
			Config: map[string]any{"recipe": "org.sample"},
		}},
	}
	recipe.SetSystemFields("tester")

	body, _ := json.Marshal(recipe)
	req := httptest.NewRequest(http.MethodPost, "/v1/recipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", resp.StatusCode)
	}
}

func TestHTTPHandlerListRecipesStorageError(t *testing.T) {
	provider := newFailingStorageProvider(withListError(errors.New("list failed")))
	app := setupHandlerWithStorageProvider(t, provider)

	req := httptest.NewRequest(http.MethodGet, "/v1/recipes", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", resp.StatusCode)
	}
}

func TestHTTPHandlerSearchRecipesMissingQuery(t *testing.T) {
	app := setupHandlerWithStorageProvider(t, newTestSeaweed())

	req := httptest.NewRequest(http.MethodGet, "/v1/recipes/search", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestHTTPHandlerSearchRecipesStorageFailure(t *testing.T) {
	provider := newFailingStorageProvider(withListError(errors.New("list failed")))
	app := setupHandlerWithStorageProvider(t, provider)

	req := httptest.NewRequest(http.MethodGet, "/v1/recipes/search?q=test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", resp.StatusCode)
	}
}

func TestHTTPHandlerRegisterRecipeRegistryUnavailable(t *testing.T) {
	handler := NewHTTPHandlerWithStorage(nil, nil, nil, nil, nil)
	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest(http.MethodPost, "/v1/recipes/register", bytes.NewBufferString(`{"recipe_class":"org.sample.Recipe","maven_coords":"g:a:v","jar_path":"jar"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", resp.StatusCode)
	}
}

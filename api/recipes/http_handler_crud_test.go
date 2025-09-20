package recipes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/api/recipes/models"
)

func TestHTTPHandler_GetRecipe_NotFound(t *testing.T) {
	handler := setupRecipesHTTPHandler(t)
	app := newAppWithHandler(handler)
	req := httptest.NewRequest(http.MethodGet, "/v1/recipes/does-not-exist", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHTTPHandler_GetRecipe_RegistryUnavailable(t *testing.T) {
	h := NewHTTPHandlerWithStorage(nil, nil, nil, nil, nil)
	app := newAppWithHandler(h)
	req := httptest.NewRequest(http.MethodGet, "/v1/recipes/any", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
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
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHTTPHandler_UpdateRecipe_InvalidJSON(t *testing.T) {
	handler := setupRecipesHTTPHandler(t)
	app := newAppWithHandler(handler)
	req := httptest.NewRequest(http.MethodPut, "/v1/recipes/test-recipe-1.0.0", bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHTTPHandler_DeleteRecipe_Success(t *testing.T) {
	handler := setupRecipesHTTPHandlerWithDeletable(t)
	// Seed a dedicated recipe to delete
	store := handler.storage.(*RegistryStorageAdapter)
	r := &models.Recipe{
		Metadata: models.RecipeMetadata{
			Name:        "del-recipe",
			Description: "desc",
			Version:     "1.0.0",
			Author:      "tester",
			Languages:   []string{"java"},
		},
		Steps: []models.RecipeStep{{Name: "s", Type: models.StepTypeOpenRewrite, Config: map[string]any{"recipe": "x"}}},
	}
	r.SetSystemFields("tester")
	if err := store.CreateRecipe(context.Background(), r); err != nil {
		t.Fatalf("seed: %v", err)
	}

	app := newAppWithHandler(handler)

	req := httptest.NewRequest(http.MethodDelete, "/v1/recipes/del-recipe-1.0.0", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

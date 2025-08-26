package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/arf/models"
)

// Mock implementations for testing
type MockEngine struct {
	recipes map[string]*models.Recipe
}

func NewMockEngine() *MockEngine {
	return &MockEngine{
		recipes: make(map[string]*models.Recipe),
	}
}

func (m *MockEngine) ExecuteRecipe(ctx context.Context, recipe *models.Recipe, codebase Codebase) (*TransformationResult, error) {
	return &TransformationResult{
		RecipeID:        recipe.ID,
		Success:         true,
		ChangesApplied:  3,
		FilesModified:   []string{"Main.java", "Utils.java"},
		ExecutionTime:   2 * time.Second,
		ValidationScore: 0.95,
	}, nil
}

func (m *MockEngine) ValidateRecipe(recipe *models.Recipe) error {
	if recipe.ID == "" {
		return &ValidationError{Message: "Recipe ID is required"}
	}
	return nil
}

func (m *MockEngine) ListAvailableRecipes() ([]*models.Recipe, error) {
	var recipes []*models.Recipe
	for _, recipe := range m.recipes {
		recipes = append(recipes, recipe)
	}
	return recipes, nil
}

func (m *MockEngine) GetRecipeMetadata(recipeID string) (*models.RecipeMetadata, error) {
	recipe, exists := m.recipes[recipeID]
	if !exists {
		return nil, &RecipeNotFoundError{RecipeID: recipeID}
	}

	return &recipe.Metadata, nil
}

func (m *MockEngine) CacheAST(key string, ast *AST) error {
	return nil
}

func (m *MockEngine) GetCachedAST(key string) (*AST, bool) {
	return nil, false
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func setupTestHandler() (*Handler, *RecipeExecutor, *MockRecipeCatalog, *MockSandboxManager) {
	// Create mock storage and sandbox manager for RecipeExecutor
	storage := NewInMemoryRecipeStorage()
	sandboxMgr := NewMockSandboxManager()
	executor := NewRecipeExecutor(storage, sandboxMgr)
	catalog := NewMockRecipeCatalog()
	benchmarkMgr := NewBenchmarkManager("./test_benchmarks")
	handler := NewHandler(executor, catalog, sandboxMgr, benchmarkMgr)

	// Add some test recipes
	testRecipe := &models.Recipe{
		ID: "test-recipe",
		Metadata: models.RecipeMetadata{
			Name:        "test-recipe",
			Description: "A test recipe",
			Author:      "test-author",
			Languages:   []string{"java"},
			Categories:  []string{"code-cleanup"},
		},
		Steps: []models.RecipeStep{{
			Name:   "cleanup-step",
			Type:   models.StepTypeOpenRewrite,
			Config: map[string]interface{}{"recipe": "org.openrewrite.java.cleanup.TestRecipe"},
		}},
	}
	storage.CreateRecipe(context.Background(), testRecipe)
	catalog.StoreRecipe(context.Background(), testRecipe)

	return handler, executor, catalog, sandboxMgr
}

func TestHandlerListRecipes(t *testing.T) {
	handler, _, _, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app)

	t.Run("list all recipes", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/arf/recipes", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var response map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&response)

		recipes, exists := response["recipes"].([]interface{})
		if !exists {
			t.Fatal("Response should contain recipes array")
		}

		if len(recipes) == 0 {
			t.Error("Expected at least one recipe")
		}
	})

	t.Run("list recipes with language filter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/arf/recipes?language=java", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})
}

func TestHandlerGetRecipe(t *testing.T) {
	handler, _, _, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app)

	t.Run("get existing recipe", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/arf/recipes/test-recipe", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var recipe models.Recipe
		json.NewDecoder(resp.Body).Decode(&recipe)

		if recipe.ID != "test-recipe" {
			t.Errorf("Expected recipe ID 'test-recipe', got %s", recipe.ID)
		}
	})

	t.Run("get non-existent recipe", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/arf/recipes/non-existent", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}
	})
}

func TestHandlerCreateRecipe(t *testing.T) {
	handler, _, _, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app)

	t.Run("create valid recipe", func(t *testing.T) {
		recipe := &models.Recipe{
			ID: "new-recipe",
			Metadata: models.RecipeMetadata{
				Name:        "new-recipe",
				Description: "A new test recipe",
				Author:      "test-author",
				Languages:   []string{"java"},
				Categories:  []string{"modernization"},
			},
			Steps: []models.RecipeStep{{
				Name:   "modernize-step",
				Type:   models.StepTypeOpenRewrite,
				Config: map[string]interface{}{"recipe": "org.openrewrite.java.modernize.NewRecipe"},
			}},
		}

		body, _ := json.Marshal(recipe)
		req := httptest.NewRequest("POST", "/v1/arf/recipes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}
	})

	t.Run("create invalid recipe", func(t *testing.T) {
		recipe := models.Recipe{
			Metadata: models.RecipeMetadata{
				Name: "invalid-recipe",
				// Missing required author field to make it invalid
			},
			// Missing required ID field
		}

		body, _ := json.Marshal(recipe)
		req := httptest.NewRequest("POST", "/v1/arf/recipes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})
}

func TestHandlerExecuteTransformation(t *testing.T) {
	handler, _, _, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app)

	t.Run("execute valid transformation", func(t *testing.T) {
		request := map[string]interface{}{
			"recipe_id": "test-recipe",
			"codebase": map[string]interface{}{
				"repository": "https://github.com/example/test-repo",
				"branch":     "main",
				"language":   "java",
			},
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/v1/arf/transform", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var result TransformationResult
		json.NewDecoder(resp.Body).Decode(&result)

		if !result.Success {
			t.Error("Expected transformation to be successful")
		}

		if result.RecipeID != "test-recipe" {
			t.Errorf("Expected recipe ID 'test-recipe', got %s", result.RecipeID)
		}
	})

	t.Run("execute with non-existent recipe", func(t *testing.T) {
		request := map[string]interface{}{
			"recipe_id": "non-existent-recipe",
			"codebase": map[string]interface{}{
				"repository": "https://github.com/example/test-repo",
				"branch":     "main",
				"language":   "java",
			},
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/v1/arf/transform", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}
	})
}

func TestHandlerSandboxOperations(t *testing.T) {
	handler, _, _, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app)

	t.Run("list sandboxes", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/arf/sandboxes", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var response map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&response)

		if _, exists := response["sandboxes"]; !exists {
			t.Error("Response should contain sandboxes array")
		}
	})

	t.Run("create sandbox", func(t *testing.T) {
		config := SandboxConfig{
			Repository:  "https://github.com/example/test-repo",
			Branch:      "main",
			Language:    "java",
			TTL:         30 * time.Minute,
			MemoryLimit: "2G",
		}

		body, _ := json.Marshal(config)
		req := httptest.NewRequest("POST", "/v1/arf/sandboxes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		var sandbox Sandbox
		json.NewDecoder(resp.Body).Decode(&sandbox)

		if sandbox.ID == "" {
			t.Error("Sandbox should have an ID")
		}

		if sandbox.Status != SandboxStatusReady {
			t.Errorf("Expected sandbox status ready, got %s", sandbox.Status)
		}
	})
}

func TestHandlerHealthCheck(t *testing.T) {
	handler, _, _, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/v1/arf/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var health map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&health)

	status, exists := health["status"].(string)
	if !exists || status != "healthy" {
		t.Errorf("Expected healthy status, got %v", health["status"])
	}

	if _, exists := health["components"]; !exists {
		t.Error("Health response should contain components")
	}
}

func TestHandlerSearchRecipes(t *testing.T) {
	handler, _, _, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app)

	t.Run("search with query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/arf/recipes/search?q=test", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var response map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&response)

		if query, exists := response["query"].(string); !exists || query != "test" {
			t.Errorf("Expected query 'test', got %v", response["query"])
		}
	})

	t.Run("search without query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/arf/recipes/search", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})
}

func TestHandlerRecipeStats(t *testing.T) {
	handler, _, catalog, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app)

	// Update stats first
	catalog.UpdateRecipeStats(context.Background(), "test-recipe", true, 2*time.Second)

	req := httptest.NewRequest("GET", "/v1/arf/recipes/test-recipe/stats", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var stats RecipeStats
	json.NewDecoder(resp.Body).Decode(&stats)

	if stats.RecipeID != "test-recipe" {
		t.Errorf("Expected recipe ID 'test-recipe', got %s", stats.RecipeID)
	}

	if stats.TotalExecutions != 1 {
		t.Errorf("Expected 1 execution, got %d", stats.TotalExecutions)
	}
}

func BenchmarkHandlerListRecipes(b *testing.B) {
	handler, _, catalog, _ := setupTestHandler()

	// Add many recipes for benchmarking
	for i := 0; i < 1000; i++ {
		recipe := &models.Recipe{
			ID: fmt.Sprintf("bench-recipe-%d", i),
			Metadata: models.RecipeMetadata{
				Name:        fmt.Sprintf("benchmark-recipe-%d", i),
				Description: fmt.Sprintf("Benchmark Recipe %d", i),
				Author:      "bench-author",
				Languages:   []string{"java"},
				Categories:  []string{"code-cleanup"},
			},
			Steps: []models.RecipeStep{{
				Name:   "bench-step",
				Type:   models.StepTypeOpenRewrite,
				Config: map[string]interface{}{"recipe": "bench"},
			}},
		}
		catalog.StoreRecipe(context.Background(), recipe)
	}

	app := fiber.New()
	handler.RegisterRoutes(app)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/v1/arf/recipes", nil)
		app.Test(req)
	}
}

func BenchmarkHandlerExecuteTransformation(b *testing.B) {
	handler, _, _, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app)

	request := map[string]interface{}{
		"recipe_id": "test-recipe",
		"codebase": map[string]interface{}{
			"repository": "https://github.com/example/test-repo",
			"branch":     "main",
			"language":   "java",
		},
	}

	body, _ := json.Marshal(request)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/v1/arf/transform", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		app.Test(req)
	}
}

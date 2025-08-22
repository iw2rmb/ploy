package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Mock implementations for testing
type MockEngine struct {
	recipes map[string]Recipe
}

func NewMockEngine() *MockEngine {
	return &MockEngine{
		recipes: make(map[string]Recipe),
	}
}

func (m *MockEngine) ExecuteRecipe(ctx context.Context, recipe Recipe, codebase Codebase) (*TransformationResult, error) {
	return &TransformationResult{
		RecipeID:        recipe.ID,
		Success:         true,
		ChangesApplied:  3,
		FilesModified:   []string{"Main.java", "Utils.java"},
		ExecutionTime:   2 * time.Second,
		ValidationScore: 0.95,
	}, nil
}

func (m *MockEngine) ValidateRecipe(recipe Recipe) error {
	if recipe.ID == "" {
		return &ValidationError{Message: "Recipe ID is required"}
	}
	return nil
}

func (m *MockEngine) ListAvailableRecipes() ([]Recipe, error) {
	var recipes []Recipe
	for _, recipe := range m.recipes {
		recipes = append(recipes, recipe)
	}
	return recipes, nil
}

func (m *MockEngine) GetRecipeMetadata(recipeID string) (*RecipeMetadata, error) {
	recipe, exists := m.recipes[recipeID]
	if !exists {
		return nil, &RecipeNotFoundError{RecipeID: recipeID}
	}

	return &RecipeMetadata{
		Recipe:              recipe,
		ApplicableLanguages: []string{recipe.Language},
		SuccessRate:         recipe.Confidence,
		CreatedAt:           time.Now().Add(-24 * time.Hour),
		UpdatedAt:           time.Now(),
	}, nil
}

func (m *MockEngine) CacheAST(key string, ast *AST) error {
	return nil
}

func (m *MockEngine) GetCachedAST(key string) (*AST, bool) {
	return nil, false
}

type MockSandboxManager struct {
	sandboxes map[string]*Sandbox
}

func NewMockSandboxManager() *MockSandboxManager {
	return &MockSandboxManager{
		sandboxes: make(map[string]*Sandbox),
	}
}

func (m *MockSandboxManager) CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error) {
	sandbox := &Sandbox{
		ID:         "test-sandbox-" + time.Now().Format("20060102150405"),
		JailName:   "test-jail",
		RootPath:   "/jail/test",
		WorkingDir: "/workspace",
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(config.TTL),
		Status:     SandboxStatusReady,
		Config:     config,
	}
	m.sandboxes[sandbox.ID] = sandbox
	return sandbox, nil
}

func (m *MockSandboxManager) DestroySandbox(ctx context.Context, sandboxID string) error {
	delete(m.sandboxes, sandboxID)
	return nil
}

func (m *MockSandboxManager) ListSandboxes(ctx context.Context) ([]SandboxInfo, error) {
	var infos []SandboxInfo
	for _, sandbox := range m.sandboxes {
		infos = append(infos, SandboxInfo{
			ID:        sandbox.ID,
			JailName:  sandbox.JailName,
			Status:    sandbox.Status,
			CreatedAt: sandbox.CreatedAt,
			ExpiresAt: sandbox.ExpiresAt,
		})
	}
	return infos, nil
}

func (m *MockSandboxManager) CleanupExpiredSandboxes(ctx context.Context) error {
	now := time.Now()
	for id, sandbox := range m.sandboxes {
		if now.After(sandbox.ExpiresAt) {
			delete(m.sandboxes, id)
		}
	}
	return nil
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func setupTestHandler() (*Handler, *MockEngine, *MockRecipeCatalog, *MockSandboxManager) {
	engine := NewMockEngine()
	catalog := NewMockRecipeCatalog()
	sandboxMgr := NewMockSandboxManager()
	handler := NewHandler(engine, catalog, sandboxMgr)

	// Add some test recipes
	testRecipe := Recipe{
		ID:          "test-recipe",
		Name:        "Test Recipe",
		Description: "A test recipe",
		Language:    "java",
		Category:    CategoryCleanup,
		Confidence:  0.9,
		Source:      "org.openrewrite.java.cleanup.TestRecipe",
	}
	engine.recipes[testRecipe.ID] = testRecipe
	catalog.StoreRecipe(context.Background(), testRecipe)

	return handler, engine, catalog, sandboxMgr
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

		var recipe Recipe
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
		recipe := Recipe{
			ID:          "new-recipe",
			Name:        "New Recipe",
			Description: "A new test recipe",
			Language:    "java",
			Category:    CategoryModernize,
			Confidence:  0.85,
			Source:      "org.openrewrite.java.modernize.NewRecipe",
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
		recipe := Recipe{
			Name: "Invalid Recipe",
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
	handler, engine, catalog, _ := setupTestHandler()

	// Add many recipes for benchmarking
	for i := 0; i < 1000; i++ {
		recipe := Recipe{
			ID:       "bench-recipe-" + string(rune(i)),
			Name:     "Benchmark Recipe " + string(rune(i)),
			Language: "java",
			Category: CategoryCleanup,
		}
		engine.recipes[recipe.ID] = recipe
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
package llms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	models "github.com/iw2rmb/ploy/internal/llms/models"
	"github.com/iw2rmb/ploy/internal/storage"
)

// MockStorage for testing API handler
type MockAPIStorage struct {
	data map[string][]byte
}

func NewMockAPIStorage() *MockAPIStorage {
	return &MockAPIStorage{
		data: make(map[string][]byte),
	}
}

func (m *MockAPIStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	data, exists := m.data[key]
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *MockAPIStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.data[key] = data
	return nil
}

func (m *MockAPIStorage) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *MockAPIStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, exists := m.data[key]
	return exists, nil
}

func (m *MockAPIStorage) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	var objects []storage.Object
	for key := range m.data {
		if strings.HasPrefix(key, opts.Prefix) {
			objects = append(objects, storage.Object{
				Key:  key,
				Size: int64(len(m.data[key])),
			})
		}
	}

	if opts.MaxKeys > 0 && len(objects) > opts.MaxKeys {
		objects = objects[:opts.MaxKeys]
	}

	return objects, nil
}

func (m *MockAPIStorage) DeleteBatch(ctx context.Context, keys []string) error {
	for _, key := range keys {
		delete(m.data, key)
	}
	return nil
}

func (m *MockAPIStorage) Head(ctx context.Context, key string) (*storage.Object, error) {
	data, exists := m.data[key]
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	return &storage.Object{
		Key:  key,
		Size: int64(len(data)),
	}, nil
}

func (m *MockAPIStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	return nil
}

func (m *MockAPIStorage) Copy(ctx context.Context, src, dst string) error {
	data, exists := m.data[src]
	if !exists {
		return fmt.Errorf("source not found")
	}
	m.data[dst] = make([]byte, len(data))
	copy(m.data[dst], data)
	return nil
}

func (m *MockAPIStorage) Move(ctx context.Context, src, dst string) error {
	if err := m.Copy(ctx, src, dst); err != nil {
		return err
	}
	return m.Delete(ctx, src)
}

func (m *MockAPIStorage) Health(ctx context.Context) error {
	return nil
}

func (m *MockAPIStorage) Metrics() *storage.StorageMetrics {
	return &storage.StorageMetrics{}
}

func TestHandler_CreateModel_Integration(t *testing.T) {
	// Setup
	mockStorage := NewMockAPIStorage()
	handler := NewHandler(mockStorage)

	app := fiber.New()
	handler.RegisterRoutes(app)

	// Test model
	testModel := models.LLMModel{
		ID:           "test-model@v1",
		Name:         "Test Model",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    8000,
	}

	modelJSON, _ := json.Marshal(testModel)

	// Test creation
	req := httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(modelJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp.StatusCode != 201 {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}

	// Test retrieval
	req = httptest.NewRequest("GET", "/v1/llms/models/test-model@v1", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandler_ListModels_Integration(t *testing.T) {
	// Setup
	mockStorage := NewMockAPIStorage()
	handler := NewHandler(mockStorage)

	app := fiber.New()
	handler.RegisterRoutes(app)

	// Create test models
	testModels := []models.LLMModel{
		{
			ID:           "openai-model@v1",
			Name:         "OpenAI Model",
			Provider:     "openai",
			Capabilities: []string{"code"},
			MaxTokens:    8000,
		},
		{
			ID:           "anthropic-model@v1",
			Name:         "Anthropic Model",
			Provider:     "anthropic",
			Capabilities: []string{"analysis"},
			MaxTokens:    200000,
		},
	}

	// Create models
	for _, model := range testModels {
		modelJSON, _ := json.Marshal(model)
		req := httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(modelJSON))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Failed to create model: %v", err)
		}
		if resp.StatusCode != 201 {
			t.Errorf("Failed to create model %s: status %d", model.ID, resp.StatusCode)
		}
	}

	// Test listing all models
	req := httptest.NewRequest("GET", "/v1/llms/models", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Test filtering by provider
	req = httptest.NewRequest("GET", "/v1/llms/models?provider=openai", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandler_UpdateAndDelete_Integration(t *testing.T) {
	// Setup
	mockStorage := NewMockAPIStorage()
	handler := NewHandler(mockStorage)

	app := fiber.New()
	handler.RegisterRoutes(app)

	// Create test model
	testModel := models.LLMModel{
		ID:           "test-model@v1",
		Name:         "Test Model",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    8000,
	}

	modelJSON, _ := json.Marshal(testModel)

	// Create model
	req := httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(modelJSON))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != 201 {
		t.Fatalf("Failed to create model: status %d", resp.StatusCode)
	}

	// Update model
	testModel.Name = "Updated Test Model"
	testModel.Capabilities = []string{"code", "analysis"}
	updatedJSON, _ := json.Marshal(testModel)

	req = httptest.NewRequest("PUT", "/v1/llms/models/test-model@v1", bytes.NewReader(updatedJSON))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200 for update, got %d", resp.StatusCode)
	}

	// Delete model
	req = httptest.NewRequest("DELETE", "/v1/llms/models/test-model@v1", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200 for delete, got %d", resp.StatusCode)
	}

	// Verify deletion
	req = httptest.NewRequest("GET", "/v1/llms/models/test-model@v1", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp.StatusCode != 404 {
		t.Errorf("Expected status 404 after deletion, got %d", resp.StatusCode)
	}
}

func TestHandler_ErrorHandling_Integration(t *testing.T) {
	// Setup
	mockStorage := NewMockAPIStorage()
	handler := NewHandler(mockStorage)

	app := fiber.New()
	handler.RegisterRoutes(app)

	// Test invalid model creation
	invalidModel := map[string]interface{}{
		"id":           "invalid", // Too short
		"name":         "Invalid Model",
		"provider":     "unknown",
		"capabilities": []string{"invalid"},
		"max_tokens":   -100,
	}

	modelJSON, _ := json.Marshal(invalidModel)
	req := httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(modelJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp.StatusCode != 400 {
		t.Errorf("Expected status 400 for invalid model, got %d", resp.StatusCode)
	}

	// Test non-existent model retrieval
	req = httptest.NewRequest("GET", "/v1/llms/models/non-existent", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp.StatusCode != 404 {
		t.Errorf("Expected status 404 for non-existent model, got %d", resp.StatusCode)
	}

	// Test duplicate model creation
	validModel := models.LLMModel{
		ID:           "duplicate-model@v1",
		Name:         "Duplicate Model",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    8000,
	}

	validJSON, _ := json.Marshal(validModel)

	// Create first model
	req = httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(validJSON))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	if resp.StatusCode != 201 {
		t.Fatalf("Failed to create first model: status %d", resp.StatusCode)
	}

	// Try to create duplicate
	req = httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(validJSON))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp.StatusCode != 409 {
		t.Errorf("Expected status 409 for duplicate model, got %d", resp.StatusCode)
	}
}

// TestHandler_ExtensiveErrorScenarios tests comprehensive error scenarios
func TestHandler_ExtensiveErrorScenarios(t *testing.T) {
	storage := NewMockAPIStorage()
	handler := NewHandler(storage)
	app := fiber.New()
	handler.RegisterRoutes(app)

	t.Run("invalid_json_body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/llms/models", strings.NewReader("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 400 {
			t.Errorf("Expected status 400 for invalid JSON, got %d", resp.StatusCode)
		}
	})

	t.Run("missing_required_fields", func(t *testing.T) {
		invalidModel := models.LLMModel{
			Name: "Incomplete Model", // Missing ID, Provider, Capabilities
		}
		modelJSON, _ := json.Marshal(invalidModel)
		req := httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(modelJSON))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 400 {
			t.Errorf("Expected status 400 for missing fields, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid_provider", func(t *testing.T) {
		invalidModel := models.LLMModel{
			ID:           "invalid@v1",
			Name:         "Invalid Provider Model",
			Provider:     "unsupported-provider",
			Capabilities: []string{"code"},
			MaxTokens:    1000,
		}
		modelJSON, _ := json.Marshal(invalidModel)
		req := httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(modelJSON))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 400 {
			t.Errorf("Expected status 400 for invalid provider, got %d", resp.StatusCode)
		}
	})

	t.Run("update_non_existent_model", func(t *testing.T) {
		model := models.LLMModel{
			ID:           "non-existent@v1",
			Name:         "Non-existent Model",
			Provider:     "openai",
			Capabilities: []string{"code"},
			MaxTokens:    1000,
		}
		modelJSON, _ := json.Marshal(model)
		req := httptest.NewRequest("PUT", "/v1/llms/models/non-existent@v1", bytes.NewReader(modelJSON))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 404 {
			t.Errorf("Expected status 404 for updating non-existent model, got %d", resp.StatusCode)
		}
	})

	t.Run("delete_non_existent_model", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/llms/models/non-existent@v1", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 404 {
			t.Errorf("Expected status 404 for deleting non-existent model, got %d", resp.StatusCode)
		}
	})
}

// TestHandler_GetModelStats_Comprehensive tests the stats endpoint thoroughly
func TestHandler_GetModelStats_Comprehensive(t *testing.T) {
	storage := NewMockAPIStorage()
	handler := NewHandler(storage)
	app := fiber.New()
	handler.RegisterRoutes(app)

	t.Run("valid_model_id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/llms/models/test-model@v1/stats", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200 for stats, got %d", resp.StatusCode)
		}

		// Parse response to check structure
		var stats map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		err = json.Unmarshal(body, &stats)
		if err != nil {
			t.Fatalf("Failed to parse stats response: %v", err)
		}

		// Check expected fields
		expectedFields := []string{"model_id", "usage_count", "total_requests", "success_rate", "cost_metrics"}
		for _, field := range expectedFields {
			if _, exists := stats[field]; !exists {
				t.Errorf("Missing field %s in stats response", field)
			}
		}
	})

	t.Run("empty_model_id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/llms/models//stats", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 400 {
			t.Errorf("Expected status 400 for empty model ID in stats, got %d", resp.StatusCode)
		}
	})
}

// TestHandler_BulkOperations tests performance with multiple models
func TestHandler_BulkOperations(t *testing.T) {
	storage := NewMockAPIStorage()
	handler := NewHandler(storage)
	app := fiber.New()
	handler.RegisterRoutes(app)

	// Create multiple models to test bulk scenarios
	const numModels = 50

	t.Run("create_multiple_models", func(t *testing.T) {
		for i := 0; i < numModels; i++ {
			model := models.LLMModel{
				ID:           fmt.Sprintf("bulk-test-%d@v1", i),
				Name:         fmt.Sprintf("Bulk Test Model %d", i),
				Provider:     "openai",
				Capabilities: []string{"code"},
				MaxTokens:    1000 + i*100,
			}

			modelJSON, _ := json.Marshal(model)
			req := httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(modelJSON))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Failed to create bulk model %d: %v", i, err)
			}
			if resp.StatusCode != 201 {
				t.Errorf("Failed to create bulk model %d: status %d", i, resp.StatusCode)
			}
		}
	})

	t.Run("list_all_models", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/llms/models", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var response map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(body, &response)

		models := response["models"].([]interface{})
		if len(models) < numModels {
			t.Errorf("Expected at least %d models, got %d", numModels, len(models))
		}
	})

	t.Run("pagination_performance", func(t *testing.T) {
		// Test pagination with smaller chunks
		limit := 10
		for offset := 0; offset < numModels; offset += limit {
			req := httptest.NewRequest("GET", fmt.Sprintf("/v1/llms/models?limit=%d&offset=%d", limit, offset), nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Pagination request failed: %v", err)
			}
			if resp.StatusCode != 200 {
				t.Errorf("Expected status 200 for pagination, got %d", resp.StatusCode)
			}
		}
	})
}

// TestHandler_ConcurrentOperations tests concurrent access patterns
func TestHandler_ConcurrentOperations(t *testing.T) {
	storage := NewMockAPIStorage()
	handler := NewHandler(storage)
	app := fiber.New()
	handler.RegisterRoutes(app)

	const numConcurrent = 10

	t.Run("concurrent_model_creation", func(t *testing.T) {
		done := make(chan bool, numConcurrent)

		for i := 0; i < numConcurrent; i++ {
			go func(id int) {
				defer func() { done <- true }()

				model := models.LLMModel{
					ID:           fmt.Sprintf("concurrent-%d@v1", id),
					Name:         fmt.Sprintf("Concurrent Model %d", id),
					Provider:     "openai",
					Capabilities: []string{"code"},
					MaxTokens:    1000,
				}

				modelJSON, _ := json.Marshal(model)
				req := httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(modelJSON))
				req.Header.Set("Content-Type", "application/json")
				resp, _ := app.Test(req)

				if resp.StatusCode != 201 {
					t.Errorf("Concurrent creation failed for model %d: status %d", id, resp.StatusCode)
				}
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < numConcurrent; i++ {
			<-done
		}
	})

	t.Run("concurrent_model_access", func(t *testing.T) {
		// Create a model first
		model := models.LLMModel{
			ID:           "concurrent-access@v1",
			Name:         "Concurrent Access Model",
			Provider:     "openai",
			Capabilities: []string{"code"},
			MaxTokens:    1000,
		}
		modelJSON, _ := json.Marshal(model)
		req := httptest.NewRequest("POST", "/v1/llms/models", bytes.NewReader(modelJSON))
		req.Header.Set("Content-Type", "application/json")
		_, _ = app.Test(req)

		done := make(chan bool, numConcurrent)

		for i := 0; i < numConcurrent; i++ {
			go func() {
				defer func() { done <- true }()

				req := httptest.NewRequest("GET", "/v1/llms/models/concurrent-access@v1", nil)
				resp, _ := app.Test(req)

				if resp.StatusCode != 200 {
					t.Errorf("Concurrent access failed: status %d", resp.StatusCode)
				}
			}()
		}

		// Wait for all goroutines
		for i := 0; i < numConcurrent; i++ {
			<-done
		}
	})
}

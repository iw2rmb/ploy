package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/arf/models"
)

// MockLLMStorage is a simple in-memory storage implementation for testing
type MockLLMStorage struct {
	data map[string][]byte
}

func NewMockLLMStorage() *MockLLMStorage {
	return &MockLLMStorage{
		data: make(map[string][]byte),
	}
}

func (m *MockLLMStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	data, exists := m.data[key]
	if !exists {
		return nil, &MockError{message: "not found"}
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *MockLLMStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...PutOption) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.data[key] = data
	return nil
}

func (m *MockLLMStorage) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *MockLLMStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, exists := m.data[key]
	return exists, nil
}

func (m *MockLLMStorage) List(ctx context.Context, opts ListOptions) ([]Object, error) {
	var objects []Object
	for key := range m.data {
		if strings.HasPrefix(key, opts.Prefix) {
			objects = append(objects, Object{
				Key:  key,
				Size: int64(len(m.data[key])),
			})
		}
	}

	// Apply MaxKeys limit
	if opts.MaxKeys > 0 && len(objects) > opts.MaxKeys {
		objects = objects[:opts.MaxKeys]
	}

	return objects, nil
}

func (m *MockLLMStorage) DeleteBatch(ctx context.Context, keys []string) error {
	for _, key := range keys {
		delete(m.data, key)
	}
	return nil
}

func (m *MockLLMStorage) Head(ctx context.Context, key string) (*Object, error) {
	data, exists := m.data[key]
	if !exists {
		return nil, &MockError{message: "not found"}
	}
	return &Object{
		Key:  key,
		Size: int64(len(data)),
	}, nil
}

func (m *MockLLMStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	return nil // Not implemented for mock
}

func (m *MockLLMStorage) Copy(ctx context.Context, src, dst string) error {
	data, exists := m.data[src]
	if !exists {
		return &MockError{message: "source not found"}
	}
	m.data[dst] = make([]byte, len(data))
	copy(m.data[dst], data)
	return nil
}

func (m *MockLLMStorage) Move(ctx context.Context, src, dst string) error {
	if err := m.Copy(ctx, src, dst); err != nil {
		return err
	}
	return m.Delete(ctx, src)
}

func (m *MockLLMStorage) Health(ctx context.Context) error {
	return nil
}

func (m *MockLLMStorage) Metrics() *StorageMetrics {
	return &StorageMetrics{} // Return empty metrics for mock
}

// MockError implements error interface for testing
type MockError struct {
	message string
}

func (e *MockError) Error() string {
	return e.message
}

func TestLLMModelStorage_CreateModel(t *testing.T) {
	mockStorage := NewMockLLMStorage()
	llmStorage := NewLLMModelStorage(mockStorage)
	ctx := context.Background()

	model := &models.LLMModel{
		ID:           "test-model@v1",
		Name:         "Test Model",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    8000,
	}

	// Test successful creation
	err := llmStorage.CreateModel(ctx, model)
	if err != nil {
		t.Errorf("CreateModel() error = %v, want nil", err)
	}

	// Verify model was stored
	exists, err := mockStorage.Exists(ctx, "llms/models/test-model@v1")
	if err != nil {
		t.Errorf("Failed to check existence: %v", err)
	}
	if !exists {
		t.Error("Expected model to exist in storage")
	}

	// Test duplicate creation
	err = llmStorage.CreateModel(ctx, model)
	if err == nil {
		t.Error("CreateModel() should fail for duplicate model")
	}
}

func TestLLMModelStorage_GetModel(t *testing.T) {
	mockStorage := NewMockLLMStorage()
	llmStorage := NewLLMModelStorage(mockStorage)
	ctx := context.Background()

	// Create a test model
	originalModel := &models.LLMModel{
		ID:           "test-model@v1",
		Name:         "Test Model",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    8000,
	}

	err := llmStorage.CreateModel(ctx, originalModel)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Test successful retrieval
	retrievedModel, err := llmStorage.GetModel(ctx, "test-model@v1")
	if err != nil {
		t.Errorf("GetModel() error = %v, want nil", err)
	}

	if retrievedModel.ID != originalModel.ID {
		t.Errorf("GetModel() ID = %s, want %s", retrievedModel.ID, originalModel.ID)
	}

	// Test retrieval of non-existent model
	_, err = llmStorage.GetModel(ctx, "non-existent@v1")
	if err == nil {
		t.Error("GetModel() should fail for non-existent model")
	}
}

func TestLLMModelStorage_UpdateModel(t *testing.T) {
	mockStorage := NewMockLLMStorage()
	llmStorage := NewLLMModelStorage(mockStorage)
	ctx := context.Background()

	// Create a test model
	originalModel := &models.LLMModel{
		ID:           "test-model@v1",
		Name:         "Test Model",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    8000,
	}

	err := llmStorage.CreateModel(ctx, originalModel)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Update the model
	updatedModel := &models.LLMModel{
		ID:           "test-model@v1",
		Name:         "Updated Test Model",
		Provider:     "openai",
		Capabilities: []string{"code", "analysis"},
		MaxTokens:    16000,
	}

	err = llmStorage.UpdateModel(ctx, updatedModel)
	if err != nil {
		t.Errorf("UpdateModel() error = %v, want nil", err)
	}

	// Verify the model was updated
	retrievedModel, err := llmStorage.GetModel(ctx, "test-model@v1")
	if err != nil {
		t.Errorf("Failed to retrieve updated model: %v", err)
	}

	if retrievedModel.Name != updatedModel.Name {
		t.Errorf("UpdateModel() Name = %s, want %s", retrievedModel.Name, updatedModel.Name)
	}

	// Test update of non-existent model
	nonExistentModel := &models.LLMModel{
		ID:           "non-existent@v1",
		Name:         "Non-existent",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    8000,
	}

	err = llmStorage.UpdateModel(ctx, nonExistentModel)
	if err == nil {
		t.Error("UpdateModel() should fail for non-existent model")
	}
}

func TestLLMModelStorage_DeleteModel(t *testing.T) {
	mockStorage := NewMockLLMStorage()
	llmStorage := NewLLMModelStorage(mockStorage)
	ctx := context.Background()

	// Create a test model
	model := &models.LLMModel{
		ID:           "test-model@v1",
		Name:         "Test Model",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    8000,
	}

	err := llmStorage.CreateModel(ctx, model)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Test successful deletion
	err = llmStorage.DeleteModel(ctx, "test-model@v1")
	if err != nil {
		t.Errorf("DeleteModel() error = %v, want nil", err)
	}

	// Verify model was deleted
	exists, err := mockStorage.Exists(ctx, "llms/models/test-model@v1")
	if err != nil {
		t.Errorf("Failed to check existence: %v", err)
	}
	if exists {
		t.Error("Expected model to be deleted from storage")
	}

	// Test deletion of non-existent model
	err = llmStorage.DeleteModel(ctx, "non-existent@v1")
	if err == nil {
		t.Error("DeleteModel() should fail for non-existent model")
	}
}

func TestLLMModelStorage_ListModels(t *testing.T) {
	mockStorage := NewMockLLMStorage()
	llmStorage := NewLLMModelStorage(mockStorage)
	ctx := context.Background()

	// Create test models
	models := []*models.LLMModel{
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
		{
			ID:           "openai-code-model@v1",
			Name:         "OpenAI Code Model",
			Provider:     "openai",
			Capabilities: []string{"code", "analysis"},
			MaxTokens:    16000,
		},
	}

	for _, model := range models {
		err := llmStorage.CreateModel(ctx, model)
		if err != nil {
			t.Fatalf("Failed to create model %s: %v", model.ID, err)
		}
	}

	// Test listing all models
	filter := ListModelFilter{Limit: 10}
	results, total, err := llmStorage.ListModels(ctx, filter)
	if err != nil {
		t.Errorf("ListModels() error = %v, want nil", err)
	}

	if len(results) != 3 {
		t.Errorf("ListModels() returned %d models, want 3", len(results))
	}

	if total != 3 {
		t.Errorf("ListModels() total = %d, want 3", total)
	}

	// Test filtering by provider
	filter = ListModelFilter{Provider: "openai", Limit: 10}
	results, _, err = llmStorage.ListModels(ctx, filter)
	if err != nil {
		t.Errorf("ListModels() with provider filter error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("ListModels() with provider filter returned %d models, want 2", len(results))
	}

	// Test filtering by capability
	filter = ListModelFilter{Capability: "analysis", Limit: 10}
	results, _, err = llmStorage.ListModels(ctx, filter)
	if err != nil {
		t.Errorf("ListModels() with capability filter error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("ListModels() with capability filter returned %d models, want 2", len(results))
	}
}

func TestLLMModelStorage_SearchModels(t *testing.T) {
	mockStorage := NewMockLLMStorage()
	llmStorage := NewLLMModelStorage(mockStorage)
	ctx := context.Background()

	// Create test models
	models := []*models.LLMModel{
		{
			ID:           "gpt-4@v1",
			Name:         "GPT-4",
			Provider:     "openai",
			Capabilities: []string{"code"},
			MaxTokens:    8000,
		},
		{
			ID:           "claude-3-sonnet@v1",
			Name:         "Claude 3 Sonnet",
			Provider:     "anthropic",
			Capabilities: []string{"analysis"},
			MaxTokens:    200000,
		},
	}

	for _, model := range models {
		err := llmStorage.CreateModel(ctx, model)
		if err != nil {
			t.Fatalf("Failed to create model %s: %v", model.ID, err)
		}
	}

	// Test search by ID
	results, err := llmStorage.SearchModels(ctx, "gpt")
	if err != nil {
		t.Errorf("SearchModels() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("SearchModels('gpt') returned %d models, want 1", len(results))
	}

	// Test search by provider
	results, err = llmStorage.SearchModels(ctx, "anthropic")
	if err != nil {
		t.Errorf("SearchModels() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("SearchModels('anthropic') returned %d models, want 1", len(results))
	}

	// Test search with no matches
	results, err = llmStorage.SearchModels(ctx, "nonexistent")
	if err != nil {
		t.Errorf("SearchModels() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("SearchModels('nonexistent') returned %d models, want 0", len(results))
	}
}

func TestLLMModelStorage_BackupAndRestore(t *testing.T) {
	mockStorage := NewMockLLMStorage()
	llmStorage := NewLLMModelStorage(mockStorage)
	ctx := context.Background()

	// Create test models
	models := []*models.LLMModel{
		{
			ID:           "model1@v1",
			Name:         "Model 1",
			Provider:     "openai",
			Capabilities: []string{"code"},
			MaxTokens:    8000,
		},
		{
			ID:           "model2@v1",
			Name:         "Model 2",
			Provider:     "anthropic",
			Capabilities: []string{"analysis"},
			MaxTokens:    200000,
		},
	}

	for _, model := range models {
		err := llmStorage.CreateModel(ctx, model)
		if err != nil {
			t.Fatalf("Failed to create model %s: %v", model.ID, err)
		}
	}

	// Test backup
	var backupBuffer bytes.Buffer
	err := llmStorage.BackupModels(ctx, &backupBuffer)
	if err != nil {
		t.Errorf("BackupModels() error = %v", err)
	}

	// Create new storage for restore test
	newMockStorage := NewMockLLMStorage()
	newLLMStorage := NewLLMModelStorage(newMockStorage)

	// Test restore
	backupReader := bytes.NewReader(backupBuffer.Bytes())
	err = newLLMStorage.RestoreModels(ctx, backupReader, false)
	if err != nil {
		t.Errorf("RestoreModels() error = %v", err)
	}

	// Verify restored models
	filter := ListModelFilter{Limit: 10}
	results, total, err := newLLMStorage.ListModels(ctx, filter)
	if err != nil {
		t.Errorf("Failed to list restored models: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("RestoreModels() restored %d models, want 2", len(results))
	}

	if total != 2 {
		t.Errorf("RestoreModels() total = %d, want 2", total)
	}
}

// TestLLMModelStorage_PerformanceTests tests performance with large numbers of models
func TestLLMModelStorage_PerformanceTests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	mockStorage := NewMockLLMStorage()
	llmStorage := NewLLMModelStorage(mockStorage)
	ctx := context.Background()

	const numModels = 500 // Reasonable number for testing

	t.Run("large_scale_operations", func(t *testing.T) {
		// Create many models
		start := time.Now()
		for i := 0; i < numModels; i++ {
			model := &models.LLMModel{
				ID:           fmt.Sprintf("perf-test-%d@v1", i),
				Name:         fmt.Sprintf("Performance Test Model %d", i),
				Provider:     "openai",
				Capabilities: []string{"code", "analysis"},
				MaxTokens:    1000 + i,
				CostPerToken: 0.001 + float64(i)*0.0001,
			}

			err := llmStorage.CreateModel(ctx, model)
			if err != nil {
				t.Fatalf("Failed to create performance model %d: %v", i, err)
			}
		}
		createDuration := time.Since(start)
		t.Logf("Created %d models in %v (avg: %v per model)",
			numModels, createDuration, createDuration/time.Duration(numModels))

		// Test listing performance
		start = time.Now()
		models, total, err := llmStorage.ListModels(ctx, ListModelFilter{})
		listDuration := time.Since(start)
		if err != nil {
			t.Fatalf("ListModels() error = %v", err)
		}
		t.Logf("Listed %d models (total: %d) in %v", len(models), total, listDuration)

		// Test search performance
		start = time.Now()
		searchResults, err := llmStorage.SearchModels(ctx, "performance")
		searchDuration := time.Since(start)
		if err != nil {
			t.Fatalf("SearchModels() error = %v", err)
		}
		t.Logf("Searched and found %d models in %v", len(searchResults), searchDuration)

		// Performance assertions
		if createDuration > time.Minute {
			t.Errorf("Creating %d models took too long: %v", numModels, createDuration)
		}
		if listDuration > time.Second*5 {
			t.Errorf("Listing %d models took too long: %v", numModels, listDuration)
		}
		if searchDuration > time.Second*2 {
			t.Errorf("Searching %d models took too long: %v", numModels, searchDuration)
		}
	})

	t.Run("concurrent_operations", func(t *testing.T) {
		const numConcurrent = 20
		var wg sync.WaitGroup
		errorsChan := make(chan error, numConcurrent)

		start := time.Now()
		for i := 0; i < numConcurrent; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				model := &models.LLMModel{
					ID:           fmt.Sprintf("concurrent-perf-%d@v1", id),
					Name:         fmt.Sprintf("Concurrent Perf Model %d", id),
					Provider:     "anthropic",
					Capabilities: []string{"reasoning"},
					MaxTokens:    2000,
				}

				if err := llmStorage.CreateModel(ctx, model); err != nil {
					errorsChan <- err
				}
			}(i)
		}

		wg.Wait()
		close(errorsChan)
		concurrentDuration := time.Since(start)

		// Check for errors
		var errors []error
		for err := range errorsChan {
			errors = append(errors, err)
		}

		if len(errors) > 0 {
			t.Errorf("Concurrent operations had %d errors: %v", len(errors), errors[0])
		}

		t.Logf("Concurrent creation of %d models took %v", numConcurrent, concurrentDuration)

		if concurrentDuration > time.Second*10 {
			t.Errorf("Concurrent operations took too long: %v", concurrentDuration)
		}
	})
}

// BenchmarkLLMModelStorage_Operations benchmarks core storage operations
func BenchmarkLLMModelStorage_Operations(b *testing.B) {
	mockStorage := NewMockLLMStorage()
	llmStorage := NewLLMModelStorage(mockStorage)
	ctx := context.Background()

	// Prepare test data
	testModel := &models.LLMModel{
		ID:           "benchmark-model@v1",
		Name:         "Benchmark Model",
		Provider:     "openai",
		Capabilities: []string{"code", "analysis"},
		MaxTokens:    4000,
		CostPerToken: 0.001,
	}

	b.Run("CreateModel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			model := *testModel
			model.ID = fmt.Sprintf("bench-create-%d@v1", i)
			_ = llmStorage.CreateModel(ctx, &model)
		}
	})

	// Create a model for other benchmarks
	_ = llmStorage.CreateModel(ctx, testModel)

	b.Run("GetModel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = llmStorage.GetModel(ctx, "benchmark-model@v1")
		}
	})

	b.Run("UpdateModel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			updatedModel := *testModel
			updatedModel.Name = fmt.Sprintf("Updated Model %d", i)
			_ = llmStorage.UpdateModel(ctx, &updatedModel)
		}
	})

	b.Run("ListModels", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, _ = llmStorage.ListModels(ctx, ListModelFilter{Limit: 50})
		}
	})

	b.Run("SearchModels", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = llmStorage.SearchModels(ctx, "benchmark")
		}
	})
}

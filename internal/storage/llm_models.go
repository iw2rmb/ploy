package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/arf/models"
)

// LLMModelStorage provides storage operations for LLM models
type LLMModelStorage struct {
	storage Storage
}

// NewLLMModelStorage creates a new LLM model storage instance
func NewLLMModelStorage(storage Storage) *LLMModelStorage {
	return &LLMModelStorage{
		storage: storage,
	}
}

// CreateModel stores a new LLM model
func (s *LLMModelStorage) CreateModel(ctx context.Context, model *models.LLMModel) error {
	key := fmt.Sprintf("llms/models/%s", model.ID)

	// Check if model already exists
	exists, err := s.storage.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check model existence: %w", err)
	}
	if exists {
		return fmt.Errorf("model already exists: %s", model.ID)
	}

	// Set system fields
	model.SetSystemFields()

	// Serialize model
	data, err := json.Marshal(model)
	if err != nil {
		return fmt.Errorf("failed to serialize model: %w", err)
	}

	// Store in storage
	reader := strings.NewReader(string(data))
	if err := s.storage.Put(ctx, key, reader, WithContentType("application/json")); err != nil {
		return fmt.Errorf("failed to store model: %w", err)
	}

	return nil
}

// GetModel retrieves an LLM model by ID
func (s *LLMModelStorage) GetModel(ctx context.Context, modelID string) (*models.LLMModel, error) {
	key := fmt.Sprintf("llms/models/%s", modelID)

	reader, err := s.storage.Get(ctx, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("model not found: %s", modelID)
		}
		return nil, fmt.Errorf("failed to get model: %w", err)
	}
	defer func() { _ = reader.Close() }()

	var model models.LLMModel
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&model); err != nil {
		return nil, fmt.Errorf("failed to parse model data: %w", err)
	}

	return &model, nil
}

// UpdateModel updates an existing LLM model
func (s *LLMModelStorage) UpdateModel(ctx context.Context, model *models.LLMModel) error {
	key := fmt.Sprintf("llms/models/%s", model.ID)

	// Check if model exists
	exists, err := s.storage.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check model existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("model not found: %s", model.ID)
	}

	// Set system fields (preserve creation time, update modified time)
	model.SetSystemFields()

	// Serialize model
	data, err := json.Marshal(model)
	if err != nil {
		return fmt.Errorf("failed to serialize model: %w", err)
	}

	// Update in storage
	reader := strings.NewReader(string(data))
	if err := s.storage.Put(ctx, key, reader, WithContentType("application/json")); err != nil {
		return fmt.Errorf("failed to update model: %w", err)
	}

	return nil
}

// DeleteModel deletes an LLM model by ID
func (s *LLMModelStorage) DeleteModel(ctx context.Context, modelID string) error {
	key := fmt.Sprintf("llms/models/%s", modelID)

	// Check if model exists
	exists, err := s.storage.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check model existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("model not found: %s", modelID)
	}

	// Delete from storage
	if err := s.storage.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}

	return nil
}

// ListModelFilter represents filter options for listing models
type ListModelFilter struct {
	Provider   string
	Capability string
	Limit      int
	Offset     int
}

// ListModels lists LLM models with optional filtering
func (s *LLMModelStorage) ListModels(ctx context.Context, filter ListModelFilter) ([]*models.LLMModel, int, error) {
	// Set default limit if not specified
	if filter.Limit <= 0 {
		filter.Limit = 20
	}

	// List objects from storage
	listOptions := ListOptions{
		Prefix:  "llms/models/",
		MaxKeys: filter.Limit + filter.Offset, // Get more to handle filtering and offset
	}

	objects, err := s.storage.List(ctx, listOptions)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list models: %w", err)
	}

	// Load and filter models
	modelsList := make([]*models.LLMModel, 0)
	processed := 0

	for _, obj := range objects {
		// Get model data
		reader, err := s.storage.Get(ctx, obj.Key)
		if err != nil {
			continue // Skip invalid models
		}

		var model models.LLMModel
		decoder := json.NewDecoder(reader)
		if err := decoder.Decode(&model); err != nil {
			_ = reader.Close()
			continue // Skip invalid models
		}
		_ = reader.Close()

		// Apply provider filter
		if filter.Provider != "" && model.Provider != filter.Provider {
			continue
		}

		// Apply capability filter
		if filter.Capability != "" && !model.HasCapability(filter.Capability) {
			continue
		}

		// Apply offset
		if processed < filter.Offset {
			processed++
			continue
		}

		// Apply limit
		if len(modelsList) >= filter.Limit {
			break
		}

		modelsList = append(modelsList, &model)
		processed++
	}

	return modelsList, len(objects), nil
}

// ModelExists checks if a model exists by ID
func (s *LLMModelStorage) ModelExists(ctx context.Context, modelID string) (bool, error) {
	key := fmt.Sprintf("llms/models/%s", modelID)
	return s.storage.Exists(ctx, key)
}

// SearchModels searches for models by name or description (simple text search)
func (s *LLMModelStorage) SearchModels(ctx context.Context, query string) ([]*models.LLMModel, error) {
	// List all models first
	filter := ListModelFilter{Limit: 1000} // Large limit for search
	allModels, _, err := s.ListModels(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list models for search: %w", err)
	}

	// Filter by query (simple case-insensitive text search)
	queryLower := strings.ToLower(query)
	results := make([]*models.LLMModel, 0)

	for _, model := range allModels {
		// Check if query matches ID, name, or provider
		if strings.Contains(strings.ToLower(model.ID), queryLower) ||
			strings.Contains(strings.ToLower(model.Name), queryLower) ||
			strings.Contains(strings.ToLower(model.Provider), queryLower) {
			results = append(results, model)
		}
	}

	return results, nil
}

// GetModelsByProvider retrieves all models for a specific provider
func (s *LLMModelStorage) GetModelsByProvider(ctx context.Context, provider string) ([]*models.LLMModel, error) {
	filter := ListModelFilter{
		Provider: provider,
		Limit:    1000, // Large limit to get all models for provider
	}

	models, _, err := s.ListModels(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list models by provider: %w", err)
	}

	return models, nil
}

// GetModelsByCapability retrieves all models with a specific capability
func (s *LLMModelStorage) GetModelsByCapability(ctx context.Context, capability string) ([]*models.LLMModel, error) {
	filter := ListModelFilter{
		Capability: capability,
		Limit:      1000, // Large limit to get all models with capability
	}

	models, _, err := s.ListModels(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list models by capability: %w", err)
	}

	return models, nil
}

// BackupModels creates a backup of all models
func (s *LLMModelStorage) BackupModels(ctx context.Context, backupWriter io.Writer) error {
	// Get all models
	filter := ListModelFilter{Limit: 10000} // Very large limit for backup
	models, _, err := s.ListModels(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list models for backup: %w", err)
	}

	// Write as JSON array
	encoder := json.NewEncoder(backupWriter)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(models); err != nil {
		return fmt.Errorf("failed to write backup data: %w", err)
	}

	return nil
}

// RestoreModels restores models from a backup
func (s *LLMModelStorage) RestoreModels(ctx context.Context, backupReader io.Reader, overwrite bool) error {
	// Decode backup data
	var models []*models.LLMModel
	decoder := json.NewDecoder(backupReader)
	if err := decoder.Decode(&models); err != nil {
		return fmt.Errorf("failed to decode backup data: %w", err)
	}

	// Restore each model
	for _, model := range models {
		// Check if model exists
		exists, err := s.ModelExists(ctx, model.ID)
		if err != nil {
			return fmt.Errorf("failed to check model existence during restore: %w", err)
		}

		if exists && !overwrite {
			continue // Skip existing models if not overwriting
		}

		// Create or update model
		if exists {
			if err := s.UpdateModel(ctx, model); err != nil {
				return fmt.Errorf("failed to update model %s during restore: %w", model.ID, err)
			}
		} else {
			if err := s.CreateModel(ctx, model); err != nil {
				return fmt.Errorf("failed to create model %s during restore: %w", model.ID, err)
			}
		}
	}

	return nil
}

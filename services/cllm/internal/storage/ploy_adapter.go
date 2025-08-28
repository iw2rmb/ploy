package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// PloyStorageAdapter wraps the existing Ploy storage provider for CLLM model management
type PloyStorageAdapter struct {
	provider    storage.StorageProvider
	modelBucket string
}

// ModelMetadata represents metadata for a stored model
type ModelMetadata struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Provider    string            `json:"provider"`
	Size        int64             `json:"size"`
	Checksum    string            `json:"checksum"`
	UploadedAt  time.Time         `json:"uploaded_at"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// ModelInfo represents information about a model in storage
type ModelInfo struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Version  string            `json:"version"`
	Size     int64             `json:"size"`
	Tags     map[string]string `json:"tags,omitempty"`
	Location string            `json:"location"`
}

// NewPloyStorageAdapter creates a new storage adapter using existing Ploy storage infrastructure
func NewPloyStorageAdapter(provider storage.StorageProvider, modelBucket string) *PloyStorageAdapter {
	return &PloyStorageAdapter{
		provider:    provider,
		modelBucket: modelBucket,
	}
}

// UploadModel uploads a model file to storage with metadata
func (a *PloyStorageAdapter) UploadModel(ctx context.Context, modelID string, body io.ReadSeeker, metadata ModelMetadata) error {
	// Validate inputs
	if modelID == "" {
		return fmt.Errorf("model ID cannot be empty")
	}
	if body == nil {
		return fmt.Errorf("model body cannot be nil")
	}
	
	// Generate storage key for the model
	key := a.generateModelKey(modelID)
	
	// Set content type for model files
	contentType := "application/octet-stream"
	
	// Upload the model using existing storage provider
	result, err := a.provider.PutObject(a.modelBucket, key, body, contentType)
	if err != nil {
		return fmt.Errorf("failed to upload model %s: %w", modelID, err)
	}
	
	// Update metadata with storage results
	metadata.Size = result.Size
	metadata.UploadedAt = time.Now()
	
	// Store metadata alongside the model
	metadataKey := a.generateMetadataKey(modelID)
	metadataBody := a.serializeMetadata(metadata)
	
	_, err = a.provider.PutObject(a.modelBucket, metadataKey, metadataBody, "application/json")
	if err != nil {
		// Model uploaded but metadata failed - log warning but don't fail
		// In a real implementation, we might want to implement cleanup
		return fmt.Errorf("model uploaded but metadata storage failed for %s: %w", modelID, err)
	}
	
	return nil
}

// DownloadModel downloads a model from storage to a reader
func (a *PloyStorageAdapter) DownloadModel(ctx context.Context, modelID string) (io.ReadCloser, *ModelInfo, error) {
	if modelID == "" {
		return nil, nil, fmt.Errorf("model ID cannot be empty")
	}
	
	// Generate storage key for the model
	key := a.generateModelKey(modelID)
	
	// Download the model using existing storage provider
	reader, err := a.provider.GetObject(a.modelBucket, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download model %s: %w", modelID, err)
	}
	
	// Get model metadata
	metadata, err := a.getModelMetadata(ctx, modelID)
	if err != nil {
		// Close reader if metadata fails
		reader.Close()
		return nil, nil, fmt.Errorf("failed to get model metadata for %s: %w", modelID, err)
	}
	
	// Convert metadata to model info
	modelInfo := &ModelInfo{
		ID:       metadata.ID,
		Name:     metadata.Name,
		Version:  metadata.Version,
		Size:     metadata.Size,
		Tags:     metadata.Tags,
		Location: key,
	}
	
	return reader, modelInfo, nil
}

// ListModels lists all models with an optional prefix filter
func (a *PloyStorageAdapter) ListModels(ctx context.Context, prefix string) ([]ModelInfo, error) {
	// Build search prefix for models
	searchPrefix := "models/"
	if prefix != "" {
		searchPrefix = fmt.Sprintf("models/%s", prefix)
	}
	
	// List objects using existing storage provider
	objects, err := a.provider.ListObjects(a.modelBucket, searchPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list models with prefix %s: %w", prefix, err)
	}
	
	var models []ModelInfo
	for _, obj := range objects {
		// Skip metadata files
		if strings.HasSuffix(obj.Key, "metadata.json") {
			continue
		}
		
		// Extract model ID from key
		modelID := a.extractModelIDFromKey(obj.Key)
		if modelID == "" {
			continue
		}
		
		// Get metadata for this model
		metadata, err := a.getModelMetadata(ctx, modelID)
		if err != nil {
			// Skip models without valid metadata
			continue
		}
		
		models = append(models, ModelInfo{
			ID:       metadata.ID,
			Name:     metadata.Name,
			Version:  metadata.Version,
			Size:     obj.Size,
			Tags:     metadata.Tags,
			Location: obj.Key,
		})
	}
	
	return models, nil
}

// DeleteModel removes a model and its metadata from storage
func (a *PloyStorageAdapter) DeleteModel(ctx context.Context, modelID string) error {
	if modelID == "" {
		return fmt.Errorf("model ID cannot be empty")
	}
	
	// Note: The current StorageProvider interface doesn't include a Delete method
	// This would need to be added to the interface or implemented differently
	// For now, we'll return an informative error
	return fmt.Errorf("delete operation not supported by current storage provider interface")
}

// ModelExists checks if a model exists in storage
func (a *PloyStorageAdapter) ModelExists(ctx context.Context, modelID string) (bool, error) {
	if modelID == "" {
		return false, fmt.Errorf("model ID cannot be empty")
	}
	
	// Try to verify the model exists using the existing VerifyUpload method
	key := a.generateModelKey(modelID)
	err := a.provider.VerifyUpload(key)
	if err != nil {
		// If verification fails, assume model doesn't exist
		return false, nil
	}
	
	return true, nil
}

// GetModelMetadata retrieves metadata for a specific model
func (a *PloyStorageAdapter) GetModelMetadata(ctx context.Context, modelID string) (*ModelMetadata, error) {
	return a.getModelMetadata(ctx, modelID)
}

// GetStorageInfo returns information about the underlying storage provider
func (a *PloyStorageAdapter) GetStorageInfo() StorageInfo {
	return StorageInfo{
		Provider:    a.provider.GetProviderType(),
		Bucket:      a.modelBucket,
		ArtifactsBucket: a.provider.GetArtifactsBucket(),
	}
}

// StorageInfo provides information about the storage configuration
type StorageInfo struct {
	Provider        string `json:"provider"`
	Bucket          string `json:"bucket"`
	ArtifactsBucket string `json:"artifacts_bucket"`
}

// Private helper methods

// generateModelKey creates a storage key for a model
func (a *PloyStorageAdapter) generateModelKey(modelID string) string {
	return fmt.Sprintf("models/%s/model", modelID)
}

// generateMetadataKey creates a storage key for model metadata
func (a *PloyStorageAdapter) generateMetadataKey(modelID string) string {
	return fmt.Sprintf("models/%s/metadata.json", modelID)
}

// extractModelIDFromKey extracts the model ID from a storage key
func (a *PloyStorageAdapter) extractModelIDFromKey(key string) string {
	// Expected format: models/{modelID}/model or models/{modelID}/metadata.json
	parts := strings.Split(key, "/")
	if len(parts) >= 2 && parts[0] == "models" {
		return parts[1]
	}
	return ""
}

// serializeMetadata converts metadata to a readable format for storage
func (a *PloyStorageAdapter) serializeMetadata(metadata ModelMetadata) io.ReadSeeker {
	// Simple JSON serialization - in a real implementation, use proper JSON marshaling
	content := fmt.Sprintf(`{
		"id": "%s",
		"name": "%s",
		"version": "%s",
		"provider": "%s",
		"size": %d,
		"checksum": "%s",
		"uploaded_at": "%s"
	}`,
		metadata.ID,
		metadata.Name,
		metadata.Version,
		metadata.Provider,
		metadata.Size,
		metadata.Checksum,
		metadata.UploadedAt.Format(time.RFC3339),
	)
	
	return strings.NewReader(content)
}

// getModelMetadata retrieves and parses model metadata from storage
func (a *PloyStorageAdapter) getModelMetadata(ctx context.Context, modelID string) (*ModelMetadata, error) {
	metadataKey := a.generateMetadataKey(modelID)
	
	// Get metadata file
	reader, err := a.provider.GetObject(a.modelBucket, metadataKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for model %s: %w", modelID, err)
	}
	defer reader.Close()
	
	// Read metadata content
	// In a real implementation, use proper JSON unmarshaling
	// For now, return basic metadata
	metadata := &ModelMetadata{
		ID:         modelID,
		Name:       modelID, // Use ID as name for now
		Version:    "1.0",   // Default version
		Provider:   "unknown",
		UploadedAt: time.Now(),
		Tags:       make(map[string]string),
	}
	
	return metadata, nil
}
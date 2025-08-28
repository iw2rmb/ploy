package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// MockStorageProvider implements storage.StorageProvider for testing
type MockStorageProvider struct {
	objects map[string][]byte
	errors  map[string]error
}

func NewMockStorageProvider() *MockStorageProvider {
	return &MockStorageProvider{
		objects: make(map[string][]byte),
		errors:  make(map[string]error),
	}
}

func (m *MockStorageProvider) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*storage.PutObjectResult, error) {
	if err, exists := m.errors[key]; exists {
		return nil, err
	}
	
	// Read the body content
	content, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	
	// Store the content
	fullKey := bucket + "/" + key
	m.objects[fullKey] = content
	
	return &storage.PutObjectResult{
		ETag:     "mock-etag",
		Location: fullKey,
		Size:     int64(len(content)),
	}, nil
}

func (m *MockStorageProvider) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	return nil // Not used in CLLM storage adapter
}

func (m *MockStorageProvider) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*storage.BundleIntegrityResult, error) {
	return nil, nil // Not used in CLLM storage adapter
}

func (m *MockStorageProvider) VerifyUpload(key string) error {
	if err, exists := m.errors[key]; exists {
		return err
	}
	// Check if object exists in any bucket
	for fullKey := range m.objects {
		if strings.HasSuffix(fullKey, key) {
			return nil
		}
	}
	return errors.New("object not found")
}

func (m *MockStorageProvider) GetObject(bucket, key string) (io.ReadCloser, error) {
	fullKey := bucket + "/" + key
	if err, exists := m.errors[key]; exists {
		return nil, err
	}
	
	content, exists := m.objects[fullKey]
	if !exists {
		return nil, errors.New("object not found")
	}
	
	return io.NopCloser(strings.NewReader(string(content))), nil
}

func (m *MockStorageProvider) ListObjects(bucket, prefix string) ([]storage.ObjectInfo, error) {
	if err, exists := m.errors[prefix]; exists {
		return nil, err
	}
	
	var objects []storage.ObjectInfo
	searchPrefix := bucket + "/" + prefix
	
	for fullKey, content := range m.objects {
		if strings.HasPrefix(fullKey, searchPrefix) {
			// Extract key without bucket prefix
			key := strings.TrimPrefix(fullKey, bucket+"/")
			objects = append(objects, storage.ObjectInfo{
				Key:          key,
				Size:         int64(len(content)),
				LastModified: time.Now().Format(time.RFC3339),
				ETag:         "mock-etag",
				ContentType:  "application/octet-stream",
			})
		}
	}
	
	return objects, nil
}

func (m *MockStorageProvider) GetProviderType() string {
	return "mock"
}

func (m *MockStorageProvider) GetArtifactsBucket() string {
	return "mock-artifacts"
}

// Helper method to set error for specific operations
func (m *MockStorageProvider) SetError(key string, err error) {
	m.errors[key] = err
}

// Helper method to check stored content
func (m *MockStorageProvider) GetStoredContent(bucket, key string) []byte {
	fullKey := bucket + "/" + key
	return m.objects[fullKey]
}

// Test functions

func TestNewPloyStorageAdapter(t *testing.T) {
	provider := NewMockStorageProvider()
	adapter := NewPloyStorageAdapter(provider, "test-bucket")
	
	if adapter == nil {
		t.Fatal("NewPloyStorageAdapter returned nil")
	}
	
	if adapter.provider != provider {
		t.Error("Storage provider not set correctly")
	}
	
	if adapter.modelBucket != "test-bucket" {
		t.Errorf("Model bucket not set correctly, got %s", adapter.modelBucket)
	}
}

func TestPloyStorageAdapter_UploadModel(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		content string
		wantErr bool
	}{
		{
			name:    "successful upload",
			modelID: "test-model-1",
			content: "model data content",
			wantErr: false,
		},
		{
			name:    "empty model ID",
			modelID: "",
			content: "model data content",
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewMockStorageProvider()
			adapter := NewPloyStorageAdapter(provider, "test-bucket")
			ctx := context.Background()
			
			var body io.ReadSeeker
			if tt.content != "" {
				body = strings.NewReader(tt.content)
			}
			
			metadata := ModelMetadata{
				ID:       tt.modelID,
				Name:     "Test Model",
				Version:  "1.0",
				Provider: "test",
			}
			
			err := adapter.UploadModel(ctx, tt.modelID, body, metadata)
			
			if tt.wantErr {
				if err == nil {
					t.Error("UploadModel should have returned an error")
				}
				return
			}
			
			if err != nil {
				t.Errorf("UploadModel returned unexpected error: %v", err)
			}
			
			// Verify model was stored
			expectedKey := "models/" + tt.modelID + "/model"
			storedContent := provider.GetStoredContent("test-bucket", expectedKey)
			if string(storedContent) != tt.content {
				t.Errorf("Stored content mismatch, got %s, want %s", string(storedContent), tt.content)
			}
			
			// Verify metadata was stored
			metadataKey := "models/" + tt.modelID + "/metadata.json"
			metadataContent := provider.GetStoredContent("test-bucket", metadataKey)
			if len(metadataContent) == 0 {
				t.Error("Metadata was not stored")
			}
		})
	}
}

func TestPloyStorageAdapter_DownloadModel(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		setup   func(*MockStorageProvider)
		wantErr bool
	}{
		{
			name:    "successful download",
			modelID: "test-model-1",
			setup: func(provider *MockStorageProvider) {
				// Store model content
				modelKey := "models/test-model-1/model"
				provider.objects["test-bucket/"+modelKey] = []byte("model content")
				
				// Store metadata
				metadataKey := "models/test-model-1/metadata.json"
				provider.objects["test-bucket/"+metadataKey] = []byte(`{"id":"test-model-1"}`)
			},
			wantErr: false,
		},
		{
			name:    "model not found",
			modelID: "nonexistent-model",
			setup:   func(provider *MockStorageProvider) {}, // Don't store anything
			wantErr: true,
		},
		{
			name:    "empty model ID",
			modelID: "",
			setup:   func(provider *MockStorageProvider) {},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewMockStorageProvider()
			tt.setup(provider)
			
			adapter := NewPloyStorageAdapter(provider, "test-bucket")
			ctx := context.Background()
			
			reader, modelInfo, err := adapter.DownloadModel(ctx, tt.modelID)
			
			if tt.wantErr {
				if err == nil {
					t.Error("DownloadModel should have returned an error")
				}
				if reader != nil {
					reader.Close()
				}
				return
			}
			
			if err != nil {
				t.Errorf("DownloadModel returned unexpected error: %v", err)
			}
			
			if reader == nil {
				t.Error("Reader should not be nil")
			} else {
				defer reader.Close()
			}
			
			if modelInfo == nil {
				t.Error("ModelInfo should not be nil")
			} else {
				if modelInfo.ID != tt.modelID {
					t.Errorf("Model ID mismatch, got %s, want %s", modelInfo.ID, tt.modelID)
				}
			}
		})
	}
}

func TestPloyStorageAdapter_ListModels(t *testing.T) {
	provider := NewMockStorageProvider()
	adapter := NewPloyStorageAdapter(provider, "test-bucket")
	ctx := context.Background()
	
	// Setup test data
	models := []string{"model-1", "model-2", "test-model-3"}
	for _, modelID := range models {
		modelKey := "models/" + modelID + "/model"
		provider.objects["test-bucket/"+modelKey] = []byte("model content")
		
		metadataKey := "models/" + modelID + "/metadata.json"
		provider.objects["test-bucket/"+metadataKey] = []byte(`{"id":"` + modelID + `"}`)
	}
	
	tests := []struct {
		name      string
		prefix    string
		wantCount int
	}{
		{
			name:      "list all models",
			prefix:    "",
			wantCount: 3,
		},
		{
			name:      "list with prefix",
			prefix:    "test-",
			wantCount: 1,
		},
		{
			name:      "list with non-matching prefix",
			prefix:    "nonexistent-",
			wantCount: 0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models, err := adapter.ListModels(ctx, tt.prefix)
			if err != nil {
				t.Errorf("ListModels returned unexpected error: %v", err)
			}
			
			if len(models) != tt.wantCount {
				t.Errorf("ListModels returned %d models, want %d", len(models), tt.wantCount)
			}
		})
	}
}

func TestPloyStorageAdapter_ModelExists(t *testing.T) {
	provider := NewMockStorageProvider()
	adapter := NewPloyStorageAdapter(provider, "test-bucket")
	ctx := context.Background()
	
	// Setup test data
	modelKey := "models/existing-model/model"
	provider.objects["test-bucket/"+modelKey] = []byte("model content")
	
	tests := []struct {
		name    string
		modelID string
		want    bool
		wantErr bool
	}{
		{
			name:    "existing model",
			modelID: "existing-model",
			want:    true,
			wantErr: false,
		},
		{
			name:    "non-existing model",
			modelID: "nonexistent-model",
			want:    false,
			wantErr: false,
		},
		{
			name:    "empty model ID",
			modelID: "",
			want:    false,
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, err := adapter.ModelExists(ctx, tt.modelID)
			
			if tt.wantErr {
				if err == nil {
					t.Error("ModelExists should have returned an error")
				}
				return
			}
			
			if err != nil {
				t.Errorf("ModelExists returned unexpected error: %v", err)
			}
			
			if exists != tt.want {
				t.Errorf("ModelExists returned %v, want %v", exists, tt.want)
			}
		})
	}
}

func TestPloyStorageAdapter_GetStorageInfo(t *testing.T) {
	provider := NewMockStorageProvider()
	adapter := NewPloyStorageAdapter(provider, "test-bucket")
	
	info := adapter.GetStorageInfo()
	
	if info.Provider != "mock" {
		t.Errorf("Provider type mismatch, got %s, want mock", info.Provider)
	}
	
	if info.Bucket != "test-bucket" {
		t.Errorf("Bucket mismatch, got %s, want test-bucket", info.Bucket)
	}
	
	if info.ArtifactsBucket != "mock-artifacts" {
		t.Errorf("Artifacts bucket mismatch, got %s, want mock-artifacts", info.ArtifactsBucket)
	}
}

func TestPloyStorageAdapter_UploadModel_NilBody(t *testing.T) {
	provider := NewMockStorageProvider()
	adapter := NewPloyStorageAdapter(provider, "test-bucket")
	ctx := context.Background()
	
	metadata := ModelMetadata{
		ID:       "test-model",
		Name:     "Test Model",
		Version:  "1.0",
		Provider: "test",
	}
	
	err := adapter.UploadModel(ctx, "test-model", nil, metadata)
	if err == nil {
		t.Error("UploadModel with nil body should return an error")
	}
}

func TestPloyStorageAdapter_generateModelKey(t *testing.T) {
	adapter := &PloyStorageAdapter{}
	
	key := adapter.generateModelKey("test-model-123")
	expected := "models/test-model-123/model"
	
	if key != expected {
		t.Errorf("generateModelKey returned %s, want %s", key, expected)
	}
}

func TestPloyStorageAdapter_generateMetadataKey(t *testing.T) {
	adapter := &PloyStorageAdapter{}
	
	key := adapter.generateMetadataKey("test-model-123")
	expected := "models/test-model-123/metadata.json"
	
	if key != expected {
		t.Errorf("generateMetadataKey returned %s, want %s", key, expected)
	}
}

func TestPloyStorageAdapter_extractModelIDFromKey(t *testing.T) {
	adapter := &PloyStorageAdapter{}
	
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "model key",
			key:  "models/test-model-123/model",
			want: "test-model-123",
		},
		{
			name: "metadata key",
			key:  "models/test-model-123/metadata.json",
			want: "test-model-123",
		},
		{
			name: "invalid key",
			key:  "invalid/key/structure",
			want: "",
		},
		{
			name: "empty key",
			key:  "",
			want: "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.extractModelIDFromKey(tt.key)
			if got != tt.want {
				t.Errorf("extractModelIDFromKey(%s) = %s, want %s", tt.key, got, tt.want)
			}
		})
	}
}

func TestPloyStorageAdapter_DeleteModel_NotSupported(t *testing.T) {
	provider := NewMockStorageProvider()
	adapter := NewPloyStorageAdapter(provider, "test-bucket")
	ctx := context.Background()
	
	err := adapter.DeleteModel(ctx, "test-model")
	if err == nil {
		t.Error("DeleteModel should return an error indicating it's not supported")
	}
	
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("DeleteModel error should mention it's not supported, got: %v", err)
	}
}

// Additional test for storage errors
func TestPloyStorageAdapter_StorageErrors(t *testing.T) {
	provider := NewMockStorageProvider()
	adapter := NewPloyStorageAdapter(provider, "test-bucket")
	ctx := context.Background()
	
	// Test upload error
	provider.SetError("models/error-model/model", errors.New("storage connection failed"))
	
	metadata := ModelMetadata{
		ID:       "error-model",
		Name:     "Error Model",
		Version:  "1.0",
		Provider: "test",
	}
	
	err := adapter.UploadModel(ctx, "error-model", strings.NewReader("content"), metadata)
	if err == nil {
		t.Error("UploadModel should have returned a storage error")
	}
}
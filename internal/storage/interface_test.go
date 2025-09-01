package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPutObjectResult(t *testing.T) {
	result := &PutObjectResult{
		ETag:     "test-etag-123",
		Location: "https://storage.example.com/bucket/key",
		Size:     1024,
	}

	assert.Equal(t, "test-etag-123", result.ETag)
	assert.Equal(t, "https://storage.example.com/bucket/key", result.Location)
	assert.Equal(t, int64(1024), result.Size)
}

func TestObjectInfo(t *testing.T) {
	objInfo := &ObjectInfo{
		Key:          "test/object/key",
		Size:         2048,
		LastModified: "2023-12-01T10:00:00Z",
		ETag:         "test-etag-456",
		ContentType:  "application/octet-stream",
	}

	assert.Equal(t, "test/object/key", objInfo.Key)
	assert.Equal(t, int64(2048), objInfo.Size)
	assert.Equal(t, "2023-12-01T10:00:00Z", objInfo.LastModified)
	assert.Equal(t, "test-etag-456", objInfo.ETag)
	assert.Equal(t, "application/octet-stream", objInfo.ContentType)
}

func TestObjectInfo_Fields(t *testing.T) {
	tests := []struct {
		name       string
		objectInfo ObjectInfo
		fieldName  string
		fieldValue interface{}
	}{
		{
			name: "key field",
			objectInfo: ObjectInfo{
				Key: "artifacts/app/v1.0.0/app.tar.gz",
			},
			fieldName:  "Key",
			fieldValue: "artifacts/app/v1.0.0/app.tar.gz",
		},
		{
			name: "size field",
			objectInfo: ObjectInfo{
				Size: 1048576, // 1MB
			},
			fieldName:  "Size",
			fieldValue: int64(1048576),
		},
		{
			name: "content type field",
			objectInfo: ObjectInfo{
				ContentType: "application/gzip",
			},
			fieldName:  "ContentType",
			fieldValue: "application/gzip",
		},
		{
			name: "etag field",
			objectInfo: ObjectInfo{
				ETag: "d41d8cd98f00b204e9800998ecf8427e",
			},
			fieldName:  "ETag",
			fieldValue: "d41d8cd98f00b204e9800998ecf8427e",
		},
		{
			name: "last modified field",
			objectInfo: ObjectInfo{
				LastModified: "2023-12-01T15:30:00Z",
			},
			fieldName:  "LastModified",
			fieldValue: "2023-12-01T15:30:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.fieldName {
			case "Key":
				assert.Equal(t, tt.fieldValue, tt.objectInfo.Key)
			case "Size":
				assert.Equal(t, tt.fieldValue, tt.objectInfo.Size)
			case "ContentType":
				assert.Equal(t, tt.fieldValue, tt.objectInfo.ContentType)
			case "ETag":
				assert.Equal(t, tt.fieldValue, tt.objectInfo.ETag)
			case "LastModified":
				assert.Equal(t, tt.fieldValue, tt.objectInfo.LastModified)
			}
		})
	}
}

func TestPutObjectResult_Fields(t *testing.T) {
	tests := []struct {
		name       string
		result     PutObjectResult
		fieldName  string
		fieldValue interface{}
	}{
		{
			name: "etag field",
			result: PutObjectResult{
				ETag: "\"33a64df551425fcc55e4d42a148795d9f25f89d4\"",
			},
			fieldName:  "ETag",
			fieldValue: "\"33a64df551425fcc55e4d42a148795d9f25f89d4\"",
		},
		{
			name: "location field",
			result: PutObjectResult{
				Location: "https://seaweedfs.local:8888/bucket/path/to/object",
			},
			fieldName:  "Location",
			fieldValue: "https://seaweedfs.local:8888/bucket/path/to/object",
		},
		{
			name: "size field",
			result: PutObjectResult{
				Size: 5242880, // 5MB
			},
			fieldName:  "Size",
			fieldValue: int64(5242880),
		},
		{
			name: "zero size",
			result: PutObjectResult{
				Size: 0,
			},
			fieldName:  "Size",
			fieldValue: int64(0),
		},
		{
			name: "empty etag",
			result: PutObjectResult{
				ETag: "",
			},
			fieldName:  "ETag",
			fieldValue: "",
		},
		{
			name: "empty location",
			result: PutObjectResult{
				Location: "",
			},
			fieldName:  "Location",
			fieldValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.fieldName {
			case "ETag":
				assert.Equal(t, tt.fieldValue, tt.result.ETag)
			case "Location":
				assert.Equal(t, tt.fieldValue, tt.result.Location)
			case "Size":
				assert.Equal(t, tt.fieldValue, tt.result.Size)
			}
		})
	}
}

// Test struct initialization patterns
func TestObjectInfo_Initialization(t *testing.T) {
	tests := []struct {
		name   string
		init   func() ObjectInfo
		verify func(*testing.T, ObjectInfo)
	}{
		{
			name: "zero value initialization",
			init: func() ObjectInfo {
				return ObjectInfo{}
			},
			verify: func(t *testing.T, obj ObjectInfo) {
				assert.Empty(t, obj.Key)
				assert.Zero(t, obj.Size)
				assert.Empty(t, obj.LastModified)
				assert.Empty(t, obj.ETag)
				assert.Empty(t, obj.ContentType)
			},
		},
		{
			name: "partial initialization",
			init: func() ObjectInfo {
				return ObjectInfo{
					Key:  "partial/key",
					Size: 1024,
				}
			},
			verify: func(t *testing.T, obj ObjectInfo) {
				assert.Equal(t, "partial/key", obj.Key)
				assert.Equal(t, int64(1024), obj.Size)
				assert.Empty(t, obj.LastModified)
				assert.Empty(t, obj.ETag)
				assert.Empty(t, obj.ContentType)
			},
		},
		{
			name: "full initialization",
			init: func() ObjectInfo {
				return ObjectInfo{
					Key:          "full/object/key",
					Size:         4096,
					LastModified: "2023-12-01T12:00:00Z",
					ETag:         "full-etag",
					ContentType:  "application/json",
				}
			},
			verify: func(t *testing.T, obj ObjectInfo) {
				assert.Equal(t, "full/object/key", obj.Key)
				assert.Equal(t, int64(4096), obj.Size)
				assert.Equal(t, "2023-12-01T12:00:00Z", obj.LastModified)
				assert.Equal(t, "full-etag", obj.ETag)
				assert.Equal(t, "application/json", obj.ContentType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.init()
			tt.verify(t, obj)
		})
	}
}

func TestPutObjectResult_Initialization(t *testing.T) {
	tests := []struct {
		name   string
		init   func() PutObjectResult
		verify func(*testing.T, PutObjectResult)
	}{
		{
			name: "zero value initialization",
			init: func() PutObjectResult {
				return PutObjectResult{}
			},
			verify: func(t *testing.T, result PutObjectResult) {
				assert.Empty(t, result.ETag)
				assert.Empty(t, result.Location)
				assert.Zero(t, result.Size)
			},
		},
		{
			name: "typical success result",
			init: func() PutObjectResult {
				return PutObjectResult{
					ETag:     "\"abc123def456\"",
					Location: "https://storage.local/bucket/key",
					Size:     2048,
				}
			},
			verify: func(t *testing.T, result PutObjectResult) {
				assert.Equal(t, "\"abc123def456\"", result.ETag)
				assert.Equal(t, "https://storage.local/bucket/key", result.Location)
				assert.Equal(t, int64(2048), result.Size)
			},
		},
		{
			name: "large file result",
			init: func() PutObjectResult {
				return PutObjectResult{
					ETag:     "\"large-file-etag\"",
					Location: "https://storage.remote/artifacts/large-app.tar.gz",
					Size:     1073741824, // 1GB
				}
			},
			verify: func(t *testing.T, result PutObjectResult) {
				assert.Equal(t, "\"large-file-etag\"", result.ETag)
				assert.Equal(t, "https://storage.remote/artifacts/large-app.tar.gz", result.Location)
				assert.Equal(t, int64(1073741824), result.Size)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.init()
			tt.verify(t, result)
		})
	}
}

// Test common usage patterns
func TestObjectInfo_CommonPatterns(t *testing.T) {
	t.Run("artifact object pattern", func(t *testing.T) {
		obj := ObjectInfo{
			Key:          "artifacts/myapp/v1.2.3/myapp-v1.2.3.tar.gz",
			Size:         10485760, // 10MB
			LastModified: "2023-12-01T14:30:00Z",
			ETag:         "\"artifact-etag-12345\"",
			ContentType:  "application/gzip",
		}

		assert.Contains(t, obj.Key, "artifacts/")
		assert.Contains(t, obj.Key, "/v1.2.3/")
		assert.Greater(t, obj.Size, int64(0))
		assert.NotEmpty(t, obj.ETag)
		assert.Equal(t, "application/gzip", obj.ContentType)
	})

	t.Run("sbom object pattern", func(t *testing.T) {
		obj := ObjectInfo{
			Key:          "artifacts/myapp/v1.2.3/myapp-v1.2.3.sbom.json",
			Size:         2048,
			LastModified: "2023-12-01T14:30:00Z",
			ETag:         "\"sbom-etag-67890\"",
			ContentType:  "application/json",
		}

		assert.Contains(t, obj.Key, ".sbom.json")
		assert.Equal(t, "application/json", obj.ContentType)
		assert.Greater(t, obj.Size, int64(0))
	})

	t.Run("signature object pattern", func(t *testing.T) {
		obj := ObjectInfo{
			Key:          "artifacts/myapp/v1.2.3/myapp-v1.2.3.sig",
			Size:         512,
			LastModified: "2023-12-01T14:30:00Z",
			ETag:         "\"sig-etag-abcde\"",
			ContentType:  "application/octet-stream",
		}

		assert.Contains(t, obj.Key, ".sig")
		assert.Greater(t, obj.Size, int64(0))
		assert.Equal(t, "application/octet-stream", obj.ContentType)
	})
}

func TestPutObjectResult_CommonPatterns(t *testing.T) {
	t.Run("small file upload result", func(t *testing.T) {
		result := PutObjectResult{
			ETag:     "\"small-file-etag\"",
			Location: "https://seaweedfs.local:8888/artifacts/config.json",
			Size:     256,
		}

		assert.NotEmpty(t, result.ETag)
		assert.Contains(t, result.Location, "artifacts/")
		assert.Less(t, result.Size, int64(1024)) // Small file
	})

	t.Run("large artifact upload result", func(t *testing.T) {
		result := PutObjectResult{
			ETag:     "\"large-artifact-etag\"",
			Location: "https://seaweedfs.local:8888/artifacts/app/v2.0.0/app-v2.0.0.tar.gz",
			Size:     104857600, // 100MB
		}

		assert.NotEmpty(t, result.ETag)
		assert.Contains(t, result.Location, "artifacts/")
		assert.Contains(t, result.Location, "v2.0.0")
		assert.Greater(t, result.Size, int64(50*1024*1024)) // Large file
	})

	t.Run("secure artifact with etag quotes", func(t *testing.T) {
		result := PutObjectResult{
			ETag:     "\"quoted-etag-with-special-chars\"",
			Location: "https://secure.storage.com/bucket/secure-app.tar.gz",
			Size:     5242880, // 5MB
		}

		assert.True(t, strings.HasPrefix(result.ETag, "\""))
		assert.True(t, strings.HasSuffix(result.ETag, "\""))
		assert.Contains(t, result.Location, "https://")
	})
}

// Test edge cases and boundary conditions
func TestObjectInfo_EdgeCases(t *testing.T) {
	t.Run("very large size", func(t *testing.T) {
		obj := ObjectInfo{
			Key:  "large-file",
			Size: 9223372036854775807, // Max int64
		}

		assert.Equal(t, int64(9223372036854775807), obj.Size)
		assert.Greater(t, obj.Size, int64(0))
	})

	t.Run("empty key with data", func(t *testing.T) {
		obj := ObjectInfo{
			Key:          "",
			Size:         1024,
			LastModified: "2023-12-01T10:00:00Z",
		}

		assert.Empty(t, obj.Key)
		assert.Greater(t, obj.Size, int64(0))
		assert.NotEmpty(t, obj.LastModified)
	})

	t.Run("unicode in key", func(t *testing.T) {
		obj := ObjectInfo{
			Key:  "artifacts/应用程序/版本1.0.0/app.tar.gz",
			Size: 1024,
		}

		assert.Contains(t, obj.Key, "应用程序")
		assert.Contains(t, obj.Key, "版本1.0.0")
	})

	t.Run("special characters in etag", func(t *testing.T) {
		obj := ObjectInfo{
			ETag: "\"etag-with-dashes_and_underscores.and.dots\"",
		}

		assert.Contains(t, obj.ETag, "-")
		assert.Contains(t, obj.ETag, "_")
		assert.Contains(t, obj.ETag, ".")
	})
}

func TestPutObjectResult_EdgeCases(t *testing.T) {
	t.Run("zero size file", func(t *testing.T) {
		result := PutObjectResult{
			ETag:     "\"empty-file-etag\"",
			Location: "https://storage.com/empty-file",
			Size:     0,
		}

		assert.Equal(t, int64(0), result.Size)
		assert.NotEmpty(t, result.ETag)
		assert.NotEmpty(t, result.Location)
	})

	t.Run("very long location URL", func(t *testing.T) {
		longURL := "https://very-long-storage-domain-name.example.com:8888/very/deep/nested/directory/structure/with/many/levels/artifacts/app/version/1.0.0/app-artifact.tar.gz"
		result := PutObjectResult{
			ETag:     "\"long-url-etag\"",
			Location: longURL,
			Size:     1024,
		}

		assert.Equal(t, longURL, result.Location)
		assert.Greater(t, len(result.Location), 100)
	})

	t.Run("etag without quotes", func(t *testing.T) {
		result := PutObjectResult{
			ETag:     "unquoted-etag-value",
			Location: "https://storage.com/file",
			Size:     512,
		}

		assert.Equal(t, "unquoted-etag-value", result.ETag)
		assert.NotContains(t, result.ETag, "\"")
	})
}

// Tests for new unified Storage interface - RED phase (failing tests)

func TestObject(t *testing.T) {
	obj := Object{
		Key:          "test/key",
		Size:         1024,
		ContentType:  "application/octet-stream",
		ETag:         "test-etag",
		LastModified: time.Date(2023, 12, 1, 10, 0, 0, 0, time.UTC),
		Metadata:     map[string]string{"version": "1.0.0"},
	}

	assert.Equal(t, "test/key", obj.Key)
	assert.Equal(t, int64(1024), obj.Size)
	assert.Equal(t, "application/octet-stream", obj.ContentType)
	assert.Equal(t, "test-etag", obj.ETag)
	assert.Equal(t, "1.0.0", obj.Metadata["version"])
}

func TestListOptions(t *testing.T) {
	opts := ListOptions{
		Prefix:     "artifacts/",
		MaxKeys:    100,
		Delimiter:  "/",
		StartAfter: "artifacts/app/v1.0.0/",
	}

	assert.Equal(t, "artifacts/", opts.Prefix)
	assert.Equal(t, 100, opts.MaxKeys)
	assert.Equal(t, "/", opts.Delimiter)
	assert.Equal(t, "artifacts/app/v1.0.0/", opts.StartAfter)
}

func TestPutOptions(t *testing.T) {
	t.Run("WithContentType", func(t *testing.T) {
		opts := &putOptions{}
		option := WithContentType("application/json")
		option(opts)

		assert.Equal(t, "application/json", opts.ContentType)
	})

	t.Run("WithMetadata", func(t *testing.T) {
		metadata := map[string]string{"version": "1.0.0", "env": "prod"}
		opts := &putOptions{}
		option := WithMetadata(metadata)
		option(opts)

		assert.Equal(t, metadata, opts.Metadata)
		assert.Equal(t, "1.0.0", opts.Metadata["version"])
		assert.Equal(t, "prod", opts.Metadata["env"])
	})

	t.Run("WithCacheControl", func(t *testing.T) {
		opts := &putOptions{}
		option := WithCacheControl("max-age=3600")
		option(opts)

		assert.Equal(t, "max-age=3600", opts.CacheControl)
	})
}

func TestStorageMetrics(t *testing.T) {
	metrics := NewStorageMetrics()
	
	// Test that we get a valid StorageMetrics instance
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalUploads)
	assert.Equal(t, int64(0), metrics.TotalDownloads)
}

// Mock implementation for testing new Storage interface
type MockStorage struct {
	objects map[string]Object
	err     error
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		objects: make(map[string]Object),
	}
}

func (m *MockStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.err != nil {
		return nil, m.err
	}
	obj, exists := m.objects[key]
	if !exists {
		return nil, NewStorageError("get", fmt.Errorf("object not found"), ErrorContext{Key: key})
	}
	return io.NopCloser(strings.NewReader(fmt.Sprintf("content-%s", obj.Key))), nil
}

func (m *MockStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...PutOption) error {
	if m.err != nil {
		return m.err
	}

	options := &putOptions{}
	for _, opt := range opts {
		opt(options)
	}

	m.objects[key] = Object{
		Key:          key,
		Size:         1024,
		ContentType:  options.ContentType,
		ETag:         "mock-etag",
		LastModified: time.Now(),
		Metadata:     options.Metadata,
	}
	return nil
}

func (m *MockStorage) Delete(ctx context.Context, key string) error {
	if m.err != nil {
		return m.err
	}
	delete(m.objects, key)
	return nil
}

func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	_, exists := m.objects[key]
	return exists, nil
}

func (m *MockStorage) List(ctx context.Context, opts ListOptions) ([]Object, error) {
	if m.err != nil {
		return nil, m.err
	}

	var objects []Object
	for _, obj := range m.objects {
		if strings.HasPrefix(obj.Key, opts.Prefix) {
			objects = append(objects, obj)
		}
	}
	return objects, nil
}

func (m *MockStorage) DeleteBatch(ctx context.Context, keys []string) error {
	if m.err != nil {
		return m.err
	}
	for _, key := range keys {
		delete(m.objects, key)
	}
	return nil
}

func (m *MockStorage) Head(ctx context.Context, key string) (*Object, error) {
	if m.err != nil {
		return nil, m.err
	}
	obj, exists := m.objects[key]
	if !exists {
		return nil, NewStorageError("head", fmt.Errorf("object not found"), ErrorContext{Key: key})
	}
	return &obj, nil
}

func (m *MockStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	if m.err != nil {
		return m.err
	}
	obj, exists := m.objects[key]
	if !exists {
		return NewStorageError("update_metadata", fmt.Errorf("object not found"), ErrorContext{Key: key})
	}
	obj.Metadata = metadata
	m.objects[key] = obj
	return nil
}

func (m *MockStorage) Copy(ctx context.Context, src, dst string) error {
	if m.err != nil {
		return m.err
	}
	obj, exists := m.objects[src]
	if !exists {
		return NewStorageError("copy", fmt.Errorf("source object not found"), ErrorContext{Key: src})
	}
	obj.Key = dst
	m.objects[dst] = obj
	return nil
}

func (m *MockStorage) Move(ctx context.Context, src, dst string) error {
	if m.err != nil {
		return m.err
	}
	if err := m.Copy(ctx, src, dst); err != nil {
		return err
	}
	delete(m.objects, src)
	return nil
}

func (m *MockStorage) Health(ctx context.Context) error {
	if m.err != nil {
		return m.err
	}
	return nil
}

func (m *MockStorage) Metrics() *StorageMetrics {
	return NewStorageMetrics()
}

func (m *MockStorage) SetError(err error) {
	m.err = err
}

// Test the new Storage interface methods
func TestStorage_Get(t *testing.T) {
	storage := NewMockStorage()
	ctx := context.Background()

	// Store an object first
	key := "test/object"
	err := storage.Put(ctx, key, strings.NewReader("test content"))
	assert.NoError(t, err)

	// Test Get
	reader, err := storage.Get(ctx, key)
	assert.NoError(t, err)
	assert.NotNil(t, reader)
	defer reader.Close()

	content, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Contains(t, string(content), key)
}

func TestStorage_Put(t *testing.T) {
	storage := NewMockStorage()
	ctx := context.Background()

	key := "test/put"
	content := "test content"

	// Test Put with options
	err := storage.Put(ctx, key, strings.NewReader(content),
		WithContentType("text/plain"),
		WithMetadata(map[string]string{"version": "1.0.0"}),
		WithCacheControl("max-age=3600"))

	assert.NoError(t, err)

	// Verify object was stored
	exists, err := storage.Exists(ctx, key)
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestStorage_Delete(t *testing.T) {
	storage := NewMockStorage()
	ctx := context.Background()

	key := "test/delete"

	// Store object first
	err := storage.Put(ctx, key, strings.NewReader("test"))
	assert.NoError(t, err)

	// Verify it exists
	exists, err := storage.Exists(ctx, key)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Delete it
	err = storage.Delete(ctx, key)
	assert.NoError(t, err)

	// Verify it's gone
	exists, err = storage.Exists(ctx, key)
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestStorage_List(t *testing.T) {
	storage := NewMockStorage()
	ctx := context.Background()

	// Store multiple objects
	keys := []string{"test/a", "test/b", "other/c"}
	for _, key := range keys {
		err := storage.Put(ctx, key, strings.NewReader("content"))
		assert.NoError(t, err)
	}

	// List with prefix
	objects, err := storage.List(ctx, ListOptions{Prefix: "test/"})
	assert.NoError(t, err)
	assert.Len(t, objects, 2)

	// Check all returned objects have the correct prefix
	for _, obj := range objects {
		assert.True(t, strings.HasPrefix(obj.Key, "test/"))
	}
}

func TestStorage_Head(t *testing.T) {
	storage := NewMockStorage()
	ctx := context.Background()

	key := "test/head"
	metadata := map[string]string{"version": "1.0.0"}

	// Store object with metadata
	err := storage.Put(ctx, key, strings.NewReader("content"),
		WithContentType("text/plain"),
		WithMetadata(metadata))
	assert.NoError(t, err)

	// Get object metadata
	obj, err := storage.Head(ctx, key)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
	assert.Equal(t, key, obj.Key)
	assert.Equal(t, "text/plain", obj.ContentType)
	assert.Equal(t, "1.0.0", obj.Metadata["version"])
}

func TestStorage_Copy(t *testing.T) {
	storage := NewMockStorage()
	ctx := context.Background()

	src := "source/object"
	dst := "destination/object"

	// Store source object
	err := storage.Put(ctx, src, strings.NewReader("content"))
	assert.NoError(t, err)

	// Copy it
	err = storage.Copy(ctx, src, dst)
	assert.NoError(t, err)

	// Verify both exist
	srcExists, err := storage.Exists(ctx, src)
	assert.NoError(t, err)
	assert.True(t, srcExists)

	dstExists, err := storage.Exists(ctx, dst)
	assert.NoError(t, err)
	assert.True(t, dstExists)
}

func TestStorage_Move(t *testing.T) {
	storage := NewMockStorage()
	ctx := context.Background()

	src := "source/object"
	dst := "destination/object"

	// Store source object
	err := storage.Put(ctx, src, strings.NewReader("content"))
	assert.NoError(t, err)

	// Move it
	err = storage.Move(ctx, src, dst)
	assert.NoError(t, err)

	// Verify source is gone and destination exists
	srcExists, err := storage.Exists(ctx, src)
	assert.NoError(t, err)
	assert.False(t, srcExists)

	dstExists, err := storage.Exists(ctx, dst)
	assert.NoError(t, err)
	assert.True(t, dstExists)
}

func TestStorage_Health(t *testing.T) {
	storage := NewMockStorage()
	ctx := context.Background()

	// Test healthy storage
	err := storage.Health(ctx)
	assert.NoError(t, err)

	// Test unhealthy storage
	storage.SetError(errors.New("storage unavailable"))
	err = storage.Health(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "storage unavailable")
}

func TestStorage_Metrics(t *testing.T) {
	storage := NewMockStorage()

	metrics := storage.Metrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalUploads)
	assert.Equal(t, int64(0), metrics.TotalDownloads)
}

package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"
)

// StorageAdapter adapts a StorageProvider to the Storage interface
type StorageAdapter struct {
	provider StorageProvider
	bucket   string
}

// NewStorageAdapter creates a new adapter for a storage provider
func NewStorageAdapter(provider StorageProvider) Storage {
	return &StorageAdapter{
		provider: provider,
		bucket:   provider.GetArtifactsBucket(),
	}
}

// Get retrieves an object from storage
func (a *StorageAdapter) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return a.provider.GetObject(a.bucket, key)
}

// Put stores an object in storage
func (a *StorageAdapter) Put(ctx context.Context, key string, reader io.Reader, opts ...PutOption) error {
	// Apply options
	options := &putOptions{
		ContentType: "application/octet-stream",
	}
	for _, opt := range opts {
		opt(options)
	}

	// Convert reader to ReadSeeker if needed
	var body io.ReadSeeker
	switch r := reader.(type) {
	case io.ReadSeeker:
		body = r
	default:
		// Read all data and create a bytes.Reader
		data, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("failed to read data: %w", err)
		}
		body = bytes.NewReader(data)
	}

	_, err := a.provider.PutObject(a.bucket, key, body, options.ContentType)
	return err
}

// Delete removes an object from storage
func (a *StorageAdapter) Delete(ctx context.Context, key string) error {
	// StorageProvider doesn't have a Delete method, so we need to implement it differently
	// For now, return an error indicating it's not implemented
	return fmt.Errorf("delete not implemented for storage provider")
}

// Exists checks if an object exists
func (a *StorageAdapter) Exists(ctx context.Context, key string) (bool, error) {
	err := a.provider.VerifyUpload(key)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// List returns objects with the given prefix
func (a *StorageAdapter) List(ctx context.Context, opts ListOptions) ([]Object, error) {
	infos, err := a.provider.ListObjects(a.bucket, opts.Prefix)
	if err != nil {
		return nil, err
	}

	// Convert ObjectInfo to Object
	objects := make([]Object, 0, len(infos))
	for _, info := range infos {
		// Parse LastModified string to time.Time
		lastModified, _ := time.Parse(time.RFC3339, info.LastModified)

		obj := Object{
			Key:          info.Key,
			Size:         info.Size,
			ContentType:  info.ContentType,
			ETag:         info.ETag,
			LastModified: lastModified,
		}
		objects = append(objects, obj)

		// Apply MaxKeys limit
		if opts.MaxKeys > 0 && len(objects) >= opts.MaxKeys {
			break
		}
	}

	return objects, nil
}

// DeleteBatch removes multiple objects
func (a *StorageAdapter) DeleteBatch(ctx context.Context, keys []string) error {
	// StorageProvider doesn't have batch delete, so iterate
	for _, key := range keys {
		if err := a.Delete(ctx, key); err != nil {
			return fmt.Errorf("failed to delete %s: %w", key, err)
		}
	}
	return nil
}

// Head returns metadata for an object
func (a *StorageAdapter) Head(ctx context.Context, key string) (*Object, error) {
	// Try to list with the exact key as prefix
	infos, err := a.provider.ListObjects(a.bucket, key)
	if err != nil {
		return nil, err
	}

	// Find exact match
	for _, info := range infos {
		if info.Key == key {
			lastModified, _ := time.Parse(time.RFC3339, info.LastModified)
			return &Object{
				Key:          info.Key,
				Size:         info.Size,
				ContentType:  info.ContentType,
				ETag:         info.ETag,
				LastModified: lastModified,
			}, nil
		}
	}

	return nil, fmt.Errorf("object not found: %s", key)
}

// UpdateMetadata updates object metadata
func (a *StorageAdapter) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	// StorageProvider doesn't support metadata update
	return fmt.Errorf("metadata update not implemented for storage provider")
}

// Copy duplicates an object
func (a *StorageAdapter) Copy(ctx context.Context, src, dst string) error {
	// Read source
	reader, err := a.Get(ctx, src)
	if err != nil {
		return fmt.Errorf("failed to read source: %w", err)
	}
	defer func() { _ = reader.Close() }()

	// Write to destination
	return a.Put(ctx, dst, reader)
}

// Move relocates an object
func (a *StorageAdapter) Move(ctx context.Context, src, dst string) error {
	// Copy then delete
	if err := a.Copy(ctx, src, dst); err != nil {
		return err
	}
	return a.Delete(ctx, src)
}

// Health checks storage health
func (a *StorageAdapter) Health(ctx context.Context) error {
	// Try to list a small prefix to check connectivity
	_, err := a.provider.ListObjects(a.bucket, "health-check-")
	return err
}

// Metrics returns storage metrics
func (a *StorageAdapter) Metrics() *StorageMetrics {
	// Return a basic metrics object
	return NewStorageMetrics()
}

// PutOptions holds options for Put operations
type PutOptions struct {
	ContentType  string
	Metadata     map[string]string
	CacheControl string
}

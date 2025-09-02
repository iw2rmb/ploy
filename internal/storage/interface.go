package storage

import (
	"context"
	"io"
	"time"
)

// PutObjectResult represents the result of a put operation
type PutObjectResult struct {
	ETag     string
	Location string
	Size     int64
}

// ObjectInfo represents information about a stored object
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified string
	ETag         string
	ContentType  string
}

// Object represents a stored object with metadata
type Object struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
	Metadata     map[string]string
}

// ListOptions configures object listing
type ListOptions struct {
	Prefix     string
	MaxKeys    int
	Delimiter  string
	StartAfter string
}

// PutOption configures Put operations
type PutOption func(*putOptions)

type putOptions struct {
	ContentType  string
	Metadata     map[string]string
	CacheControl string
}

func WithContentType(ct string) PutOption {
	return func(opts *putOptions) {
		opts.ContentType = ct
	}
}

func WithMetadata(m map[string]string) PutOption {
	return func(opts *putOptions) {
		opts.Metadata = m
	}
}

func WithCacheControl(cc string) PutOption {
	return func(opts *putOptions) {
		opts.CacheControl = cc
	}
}

// Storage defines the core unified storage interface
type Storage interface {
	// Basic operations
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Put(ctx context.Context, key string, reader io.Reader, opts ...PutOption) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)

	// Batch operations
	List(ctx context.Context, opts ListOptions) ([]Object, error)
	DeleteBatch(ctx context.Context, keys []string) error

	// Metadata operations
	Head(ctx context.Context, key string) (*Object, error)
	UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error

	// Advanced operations
	Copy(ctx context.Context, src, dst string) error
	Move(ctx context.Context, src, dst string) error

	// Health and metrics
	Health(ctx context.Context) error
	Metrics() *StorageMetrics // Use existing StorageMetrics from monitoring.go
}

// Note: StorageMetrics is now defined above with comprehensive functionality

// StorageProvider interface is kept for compatibility but only implemented by SeaweedFSClient
type StorageProvider interface {
	// PutObject uploads a single object to storage
	PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error)

	// UploadArtifactBundle uploads an artifact and all its related files (SBOM, signature, certificate)
	UploadArtifactBundle(keyPrefix, artifactPath string) error

	// UploadArtifactBundleWithVerification uploads and verifies integrity of artifact bundle
	UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*BundleIntegrityResult, error)

	// VerifyUpload checks if an object exists in storage
	VerifyUpload(key string) error

	// GetObject retrieves an object from storage
	GetObject(bucket, key string) (io.ReadCloser, error)

	// ListObjects lists objects with a given prefix
	ListObjects(bucket, prefix string) ([]ObjectInfo, error)

	// GetProviderType returns the storage provider type
	GetProviderType() string

	// GetArtifactsBucket returns the artifacts bucket name
	GetArtifactsBucket() string
}

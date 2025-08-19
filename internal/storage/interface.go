package storage

import (
	"io"
)

// StorageProvider defines the interface for different storage backends
type StorageProvider interface {
	// PutObject uploads a single object to storage
	PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error)
	
	// UploadArtifactBundle uploads an artifact and all its related files (SBOM, signature, certificate)
	UploadArtifactBundle(keyPrefix, artifactPath string) error
	
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

// StorageMetrics provides storage operation metrics
type StorageMetrics struct {
	TotalUploads   int64
	FailedUploads  int64
	TotalDownloads int64
	FailedDownloads int64
	TotalSize      int64
}
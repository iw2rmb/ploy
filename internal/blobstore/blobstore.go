// Package blobstore provides an interface for object storage operations.
package blobstore

import (
	"context"
	"io"
)

// Store defines the interface for blob storage operations.
type Store interface {
	// Put uploads data to the store at the given key.
	// Returns the etag of the uploaded object.
	Put(ctx context.Context, key, contentType string, data []byte) (etag string, err error)

	// Get retrieves an object from the store.
	// Caller is responsible for closing the returned ReadCloser.
	Get(ctx context.Context, key string) (rc io.ReadCloser, size int64, err error)

	// Delete removes an object from the store.
	Delete(ctx context.Context, key string) error
}

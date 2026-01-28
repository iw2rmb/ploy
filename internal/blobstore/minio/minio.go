// Package minio provides a MinIO/S3-compatible implementation of blobstore.Store.
package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/iw2rmb/ploy/internal/blobstore"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// Store implements blobstore.Store using MinIO.
type Store struct {
	client *minio.Client
	bucket string
}

// Ensure Store implements blobstore.Store.
var _ blobstore.Store = (*Store)(nil)

// New creates a new MinIO blobstore.Store.
func New(cfg config.ObjectStoreConfig) (*Store, error) {
	// Strip protocol prefix from endpoint if present.
	// MinIO client expects host:port format, not URL format.
	endpoint := cfg.Endpoint
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.Secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio: create client: %w", err)
	}

	return &Store{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// Put uploads data to MinIO at the given key.
func (s *Store) Put(ctx context.Context, key, contentType string, data []byte) (string, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("minio: put object %s: %w", key, err)
	}
	return info.ETag, nil
}

// Get retrieves an object from MinIO.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("minio: get object %s: %w", key, err)
	}

	stat, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, 0, fmt.Errorf("minio: stat object %s: %w", key, err)
	}

	return obj, stat.Size, nil
}

// Delete removes an object from MinIO.
func (s *Store) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("minio: delete object %s: %w", key, err)
	}
	return nil
}

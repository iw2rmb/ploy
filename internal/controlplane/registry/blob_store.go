package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// BlobDocument captures metadata for a single blob digest.
type BlobDocument struct {
	Repo             string    `json:"repo"`
	Digest           string    `json:"digest"`
	MediaType        string    `json:"media_type"`
	Size             int64     `json:"size"`
	CID              string    `json:"cid"`
	Status           string    `json:"status"`
	PinState         PinState  `json:"pin_state,omitempty"`
	PinReplicas      int       `json:"pin_replicas,omitempty"`
	PinRetryCount    int       `json:"pin_retry_count,omitempty"`
	PinError         string    `json:"pin_error,omitempty"`
	PinUpdatedAt     time.Time `json:"pin_updated_at,omitempty"`
	PinNextAttemptAt time.Time `json:"pin_next_attempt_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	DeletedAt        time.Time `json:"deleted_at,omitempty"`
}

// PutBlob inserts or updates blob metadata for the specified repository.
func (s *Store) PutBlob(ctx context.Context, blob BlobDocument) (BlobDocument, error) {
	if err := s.ensureClient(); err != nil {
		return BlobDocument{}, err
	}
	repo, digest, err := s.validateRepoDigest(blob.Repo, blob.Digest)
	if err != nil {
		return BlobDocument{}, err
	}
	now := s.clock().UTC()
	existing, err := s.readBlob(ctx, repo, digest)
	blobExists := err == nil
	if err == nil {
		blob.CreatedAt = existing.CreatedAt
	} else if errors.Is(err, ErrBlobNotFound) {
		blob.CreatedAt = now
	} else {
		return BlobDocument{}, err
	}
	blob.Repo = repo
	blob.Digest = digest
	blob.UpdatedAt = now
	blob.DeletedAt = time.Time{}
	if strings.TrimSpace(blob.Status) == "" {
		blob.Status = BlobStatusAvailable
	}
	if blobExists {
		blob.PinState = existing.PinState
		blob.PinReplicas = existing.PinReplicas
		blob.PinRetryCount = existing.PinRetryCount
		blob.PinError = existing.PinError
		blob.PinUpdatedAt = existing.PinUpdatedAt
		blob.PinNextAttemptAt = existing.PinNextAttemptAt
	} else {
		if strings.TrimSpace(string(blob.PinState)) == "" {
			blob.PinState = PinStateQueued
		}
		if blob.PinRetryCount < 0 {
			blob.PinRetryCount = 0
		}
		if blob.PinUpdatedAt.IsZero() {
			blob.PinUpdatedAt = now
		}
		blob.PinNextAttemptAt = time.Time{}
	}
	payload, err := json.Marshal(blob)
	if err != nil {
		return BlobDocument{}, fmt.Errorf("registry: encode blob %s: %w", digest, err)
	}
	if _, err := s.client.Put(ctx, s.blobKey(repo, digest), string(payload)); err != nil {
		return BlobDocument{}, fmt.Errorf("registry: persist blob %s: %w", digest, err)
	}
	return blob, nil
}

// GetBlob fetches blob metadata.
func (s *Store) GetBlob(ctx context.Context, repo, digest string) (BlobDocument, error) {
	if err := s.ensureClient(); err != nil {
		return BlobDocument{}, err
	}
	repo, digest, err := s.validateRepoDigest(repo, digest)
	if err != nil {
		return BlobDocument{}, err
	}
	return s.readBlob(ctx, repo, digest)
}

// DeleteBlob marks blob metadata as deleted.
func (s *Store) DeleteBlob(ctx context.Context, repo, digest string) (BlobDocument, error) {
	if err := s.ensureClient(); err != nil {
		return BlobDocument{}, err
	}
	repo, digest, err := s.validateRepoDigest(repo, digest)
	if err != nil {
		return BlobDocument{}, err
	}
	record, err := s.readBlob(ctx, repo, digest)
	if err != nil {
		return BlobDocument{}, err
	}
	record.Status = BlobStatusDeleted
	record.UpdatedAt = s.clock().UTC()
	record.DeletedAt = record.UpdatedAt
	payload, err := json.Marshal(record)
	if err != nil {
		return BlobDocument{}, fmt.Errorf("registry: encode blob delete %s: %w", digest, err)
	}
	if _, err := s.client.Put(ctx, s.blobKey(repo, digest), string(payload)); err != nil {
		return BlobDocument{}, fmt.Errorf("registry: delete blob %s: %w", digest, err)
	}
	return record, nil
}

// UpdateBlobPinState mutates the pin metadata for an existing blob document.
func (s *Store) UpdateBlobPinState(ctx context.Context, repo, digest string, update PinStateUpdate) (BlobDocument, error) {
	if err := s.ensureClient(); err != nil {
		return BlobDocument{}, err
	}
	repo, digest, err := s.validateRepoDigest(repo, digest)
	if err != nil {
		return BlobDocument{}, err
	}
	if strings.TrimSpace(string(update.State)) == "" {
		return BlobDocument{}, errors.New("registry: pin state required")
	}
	record, err := s.readBlob(ctx, repo, digest)
	if err != nil {
		return BlobDocument{}, err
	}
	now := s.clock().UTC()
	record.PinState = update.State
	if update.Replicas != nil {
		record.PinReplicas = *update.Replicas
	}
	if update.RetryCountDelta != 0 {
		record.PinRetryCount += update.RetryCountDelta
		if record.PinRetryCount < 0 {
			record.PinRetryCount = 0
		}
	}
	record.PinError = strings.TrimSpace(update.Error)
	if update.NextAttemptAt.IsZero() {
		record.PinNextAttemptAt = time.Time{}
	} else {
		record.PinNextAttemptAt = update.NextAttemptAt.UTC()
	}
	record.PinUpdatedAt = now
	record.UpdatedAt = now

	payload, err := json.Marshal(record)
	if err != nil {
		return BlobDocument{}, fmt.Errorf("registry: encode blob pin state %s: %w", digest, err)
	}
	if _, err := s.client.Put(ctx, s.blobKey(repo, digest), string(payload)); err != nil {
		return BlobDocument{}, fmt.Errorf("registry: update blob pin state %s: %w", digest, err)
	}
	return record, nil
}

func (s *Store) readBlob(ctx context.Context, repo, digest string) (BlobDocument, error) {
	resp, err := s.client.Get(ctx, s.blobKey(repo, digest))
	if err != nil {
		return BlobDocument{}, fmt.Errorf("registry: get blob %s: %w", digest, err)
	}
	if len(resp.Kvs) == 0 {
		return BlobDocument{}, ErrBlobNotFound
	}
	var doc BlobDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return BlobDocument{}, fmt.Errorf("registry: decode blob %s: %w", digest, err)
	}
	if doc.Status == BlobStatusDeleted || !doc.DeletedAt.IsZero() {
		return BlobDocument{}, ErrBlobNotFound
	}
	return doc, nil
}

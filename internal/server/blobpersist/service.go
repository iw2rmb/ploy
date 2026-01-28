// Package blobpersist provides a service for persisting blob data with coordinated
// database metadata and object storage writes.
package blobpersist

import (
	"context"
	"errors"
	"fmt"

	"github.com/iw2rmb/ploy/internal/blobstore"
	"github.com/iw2rmb/ploy/internal/store"
)

// Service coordinates database metadata and object storage writes.
type Service struct {
	store     store.Store
	blobstore blobstore.Store
}

// New creates a new blobpersist service.
func New(st store.Store, bs blobstore.Store) *Service {
	return &Service{
		store:     st,
		blobstore: bs,
	}
}

func (s *Service) validate() error {
	if s == nil {
		return errors.New("blobpersist: service is nil")
	}
	if s.store == nil {
		return errors.New("blobpersist: store is nil")
	}
	if s.blobstore == nil {
		return errors.New("blobpersist: blobstore is nil")
	}
	return nil
}

// CreateLog creates a log entry in the database and uploads the data to object storage.
// Returns the created log metadata.
func (s *Service) CreateLog(ctx context.Context, params store.CreateLogParams, data []byte) (store.Log, error) {
	if err := s.validate(); err != nil {
		return store.Log{}, err
	}

	// Set the data size from the actual data length.
	params.DataSize = int64(len(data))

	// Insert metadata row to DB (returns row with generated object_key).
	log, err := s.store.CreateLog(ctx, params)
	if err != nil {
		return store.Log{}, fmt.Errorf("blobpersist: create log metadata: %w", err)
	}

	// Upload bytes to MinIO at object_key.
	if log.ObjectKey == nil || *log.ObjectKey == "" {
		return store.Log{}, fmt.Errorf("blobpersist: log %d has no object_key", log.ID)
	}

	_, err = s.blobstore.Put(ctx, *log.ObjectKey, "application/gzip", data)
	if err != nil {
		rollbackErr := s.store.DeleteLog(ctx, log.ID)
		if rollbackErr != nil {
			return store.Log{}, fmt.Errorf("blobpersist: upload log %d: %w (rollback failed: %v)", log.ID, err, rollbackErr)
		}
		return store.Log{}, fmt.Errorf("blobpersist: upload log %d: %w", log.ID, err)
	}

	return log, nil
}

// CreateDiff creates a diff entry in the database and uploads the patch to object storage.
// Returns the created diff metadata.
func (s *Service) CreateDiff(ctx context.Context, params store.CreateDiffParams, patch []byte) (store.Diff, error) {
	if err := s.validate(); err != nil {
		return store.Diff{}, err
	}

	// Set the patch size from the actual data length.
	params.PatchSize = int64(len(patch))

	// Insert metadata row to DB (returns row with generated object_key).
	diff, err := s.store.CreateDiff(ctx, params)
	if err != nil {
		return store.Diff{}, fmt.Errorf("blobpersist: create diff metadata: %w", err)
	}

	// Upload bytes to MinIO at object_key.
	if diff.ObjectKey == nil || *diff.ObjectKey == "" {
		return store.Diff{}, fmt.Errorf("blobpersist: diff %v has no object_key", diff.ID)
	}

	_, err = s.blobstore.Put(ctx, *diff.ObjectKey, "application/gzip", patch)
	if err != nil {
		rollbackErr := s.store.DeleteDiff(ctx, diff.ID)
		if rollbackErr != nil {
			return store.Diff{}, fmt.Errorf("blobpersist: upload diff %v: %w (rollback failed: %v)", diff.ID, err, rollbackErr)
		}
		return store.Diff{}, fmt.Errorf("blobpersist: upload diff %v: %w", diff.ID, err)
	}

	return diff, nil
}

// CreateArtifactBundle creates an artifact bundle entry in the database and uploads the bundle to object storage.
// Returns the created artifact bundle metadata.
func (s *Service) CreateArtifactBundle(ctx context.Context, params store.CreateArtifactBundleParams, bundle []byte) (store.ArtifactBundle, error) {
	if err := s.validate(); err != nil {
		return store.ArtifactBundle{}, err
	}

	// Set the bundle size from the actual data length.
	params.BundleSize = int64(len(bundle))

	// Insert metadata row to DB (returns row with generated object_key).
	artifact, err := s.store.CreateArtifactBundle(ctx, params)
	if err != nil {
		return store.ArtifactBundle{}, fmt.Errorf("blobpersist: create artifact bundle metadata: %w", err)
	}

	// Upload bytes to MinIO at object_key.
	if artifact.ObjectKey == nil || *artifact.ObjectKey == "" {
		return store.ArtifactBundle{}, fmt.Errorf("blobpersist: artifact bundle %v has no object_key", artifact.ID)
	}

	_, err = s.blobstore.Put(ctx, *artifact.ObjectKey, "application/gzip", bundle)
	if err != nil {
		rollbackErr := s.store.DeleteArtifactBundle(ctx, artifact.ID)
		if rollbackErr != nil {
			return store.ArtifactBundle{}, fmt.Errorf("blobpersist: upload artifact bundle %v: %w (rollback failed: %v)", artifact.ID, err, rollbackErr)
		}
		return store.ArtifactBundle{}, fmt.Errorf("blobpersist: upload artifact bundle %v: %w", artifact.ID, err)
	}

	return artifact, nil
}

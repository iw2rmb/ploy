// Package blobpersist provides a service for persisting blob data with coordinated
// database metadata and object storage writes.
package blobpersist

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/iw2rmb/ploy/internal/blobstore"
	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

var (
	ErrRecoveryArtifactNotFound    = errors.New("recovery artifact not found")
	ErrRecoveryArtifactUnreadable  = errors.New("recovery artifact unreadable")
	ErrRecoveryArtifactInvalidJSON = errors.New("recovery artifact invalid json payload")
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

// persistBlob is the shared insert→upload→rollback helper.
func persistBlob[T any](
	ctx context.Context,
	s *Service,
	data []byte,
	setSize func(int64),
	insert func(context.Context) (T, error),
	objectKey func(T) *string,
	entityID func(T) any,
	entityName string,
	deleteFn func(context.Context, T) error,
) (T, error) {
	var zero T
	if err := s.validate(); err != nil {
		return zero, err
	}

	setSize(int64(len(data)))

	row, err := insert(ctx)
	if err != nil {
		return zero, fmt.Errorf("blobpersist: create %s metadata: %w", entityName, err)
	}

	key := objectKey(row)
	if key == nil || *key == "" {
		return zero, fmt.Errorf("blobpersist: %s %v has no object_key", entityName, entityID(row))
	}

	_, err = s.blobstore.Put(ctx, *key, "application/gzip", data)
	if err != nil {
		rollbackErr := deleteFn(ctx, row)
		if rollbackErr != nil {
			return zero, fmt.Errorf("blobpersist: upload %s %v: %w (rollback failed: %v)", entityName, entityID(row), err, rollbackErr)
		}
		return zero, fmt.Errorf("blobpersist: upload %s %v: %w", entityName, entityID(row), err)
	}

	return row, nil
}

// CreateLog creates a log entry in the database and uploads the data to object storage.
func (s *Service) CreateLog(ctx context.Context, params store.CreateLogParams, data []byte) (store.Log, error) {
	return persistBlob(ctx, s, data,
		func(size int64) { params.DataSize = size },
		func(ctx context.Context) (store.Log, error) { return s.store.CreateLog(ctx, params) },
		func(row store.Log) *string { return row.ObjectKey },
		func(row store.Log) any { return row.ID },
		"log",
		func(ctx context.Context, row store.Log) error { return s.store.DeleteLog(ctx, row.ID) },
	)
}

// CreateDiff creates a diff entry in the database and uploads the patch to object storage.
func (s *Service) CreateDiff(ctx context.Context, params store.CreateDiffParams, patch []byte) (store.Diff, error) {
	return persistBlob(ctx, s, patch,
		func(size int64) { params.PatchSize = size },
		func(ctx context.Context) (store.Diff, error) { return s.store.CreateDiff(ctx, params) },
		func(row store.Diff) *string { return row.ObjectKey },
		func(row store.Diff) any { return row.ID },
		"diff",
		func(ctx context.Context, row store.Diff) error { return s.store.DeleteDiff(ctx, row.ID) },
	)
}

// CreateArtifactBundle creates an artifact bundle entry in the database and uploads the bundle to object storage.
func (s *Service) CreateArtifactBundle(ctx context.Context, params store.CreateArtifactBundleParams, bundle []byte) (store.ArtifactBundle, error) {
	return persistBlob(ctx, s, bundle,
		func(size int64) { params.BundleSize = size },
		func(ctx context.Context) (store.ArtifactBundle, error) {
			return s.store.CreateArtifactBundle(ctx, params)
		},
		func(row store.ArtifactBundle) *string { return row.ObjectKey },
		func(row store.ArtifactBundle) any { return row.ID },
		"artifact bundle",
		func(ctx context.Context, row store.ArtifactBundle) error {
			return s.store.DeleteArtifactBundle(ctx, row.ID)
		},
	)
}

// LoadRecoveryArtifact resolves and reads a specific artifact path from persisted
// job artifact bundles. expectedPath must use absolute wire form (for example
// "/out/gate-profile-candidate.json").
func (s *Service) LoadRecoveryArtifact(ctx context.Context, runID types.RunID, jobID types.JobID, expectedPath string) ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	canonicalPath, err := canonicalRecoveryArtifactPath(expectedPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecoveryArtifactUnreadable, err)
	}

	bundles, err := s.store.ListArtifactBundlesMetaByRunAndJob(ctx, store.ListArtifactBundlesMetaByRunAndJobParams{
		RunID: runID,
		JobID: &jobID,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: list artifact bundles: %v", ErrRecoveryArtifactUnreadable, err)
	}

	var firstUnreadable error
	for _, bundle := range bundles {
		if bundle.ObjectKey == nil || strings.TrimSpace(*bundle.ObjectKey) == "" {
			if firstUnreadable == nil {
				firstUnreadable = fmt.Errorf("bundle %x has empty object key", bundle.ID.Bytes)
			}
			continue
		}

		rc, _, getErr := s.blobstore.Get(ctx, *bundle.ObjectKey)
		if getErr != nil {
			if firstUnreadable == nil {
				firstUnreadable = fmt.Errorf("get object %q: %w", *bundle.ObjectKey, getErr)
			}
			continue
		}

		raw, found, readErr := readArtifactFromTarGz(rc, canonicalPath)
		_ = rc.Close()
		if readErr != nil {
			if firstUnreadable == nil {
				firstUnreadable = fmt.Errorf("read bundle %q: %w", *bundle.ObjectKey, readErr)
			}
			continue
		}
		if !found {
			continue
		}

		if !json.Valid(raw) {
			return nil, fmt.Errorf("%w: path=%s", ErrRecoveryArtifactInvalidJSON, expectedPath)
		}
		return raw, nil
	}

	if firstUnreadable != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecoveryArtifactUnreadable, firstUnreadable)
	}
	return nil, fmt.Errorf("%w: path=%s", ErrRecoveryArtifactNotFound, expectedPath)
}

func canonicalRecoveryArtifactPath(expectedPath string) (string, error) {
	p := strings.TrimSpace(expectedPath)
	if p == "" {
		return "", fmt.Errorf("expected artifact path is required")
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(p, "/"))
	if cleaned == "/" || strings.HasPrefix(cleaned, "/../") {
		return "", fmt.Errorf("invalid expected artifact path %q", expectedPath)
	}
	return strings.TrimPrefix(cleaned, "/"), nil
}

func normalizeTarEntryPath(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(n, "/"))
	if cleaned == "/" || strings.HasPrefix(cleaned, "/../") {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

func readArtifactFromTarGz(r io.Reader, expectedEntry string) ([]byte, bool, error) {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return nil, false, fmt.Errorf("open gzip: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	tr := tar.NewReader(gzReader)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("read tar entry: %w", err)
		}
		if hdr == nil || hdr.Typeflag == tar.TypeDir {
			continue
		}
		if normalizeTarEntryPath(hdr.Name) != expectedEntry {
			continue
		}
		data, readErr := io.ReadAll(tr)
		if readErr != nil {
			return nil, false, fmt.Errorf("read tar payload: %w", readErr)
		}
		return bytes.TrimSpace(data), true, nil
	}
}

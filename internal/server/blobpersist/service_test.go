package blobpersist

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/blobstore"
	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type stubStore struct {
	store.Store

	createLog            func(ctx context.Context, arg store.CreateLogParams) (store.Log, error)
	deleteLog            func(ctx context.Context, id int64) error
	createDiff           func(ctx context.Context, arg store.CreateDiffParams) (store.Diff, error)
	deleteDiff           func(ctx context.Context, id pgtype.UUID) error
	createArtifactBundle func(ctx context.Context, arg store.CreateArtifactBundleParams) (store.ArtifactBundle, error)
	deleteArtifactBundle func(ctx context.Context, id pgtype.UUID) error
}

func (s *stubStore) CreateLog(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
	return s.createLog(ctx, arg)
}

func (s *stubStore) DeleteLog(ctx context.Context, id int64) error {
	return s.deleteLog(ctx, id)
}

func (s *stubStore) CreateDiff(ctx context.Context, arg store.CreateDiffParams) (store.Diff, error) {
	return s.createDiff(ctx, arg)
}

func (s *stubStore) DeleteDiff(ctx context.Context, id pgtype.UUID) error {
	return s.deleteDiff(ctx, id)
}

func (s *stubStore) CreateArtifactBundle(ctx context.Context, arg store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
	return s.createArtifactBundle(ctx, arg)
}

func (s *stubStore) DeleteArtifactBundle(ctx context.Context, id pgtype.UUID) error {
	return s.deleteArtifactBundle(ctx, id)
}

type stubBlobstore struct {
	put func(ctx context.Context, key, contentType string, data []byte) (string, error)
}

var _ blobstore.Store = (*stubBlobstore)(nil)

func (s *stubBlobstore) Put(ctx context.Context, key, contentType string, data []byte) (string, error) {
	return s.put(ctx, key, contentType, data)
}

func (s *stubBlobstore) Get(context.Context, string) (io.ReadCloser, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (s *stubBlobstore) Delete(context.Context, string) error {
	return errors.New("not implemented")
}

func TestCreateLog_UploadsAndReturnsMetadata(t *testing.T) {
	data := []byte("gzipped-log")
	runID := types.NewRunID()
	objKey := "logs/run/" + runID.String() + "/job/none/chunk/1/log/10.gz"

	var gotDataSize int64
	var putCalled bool

	st := &stubStore{
		createLog: func(_ context.Context, arg store.CreateLogParams) (store.Log, error) {
			gotDataSize = arg.DataSize
			return store.Log{ID: 10, RunID: arg.RunID, JobID: arg.JobID, ChunkNo: arg.ChunkNo, DataSize: arg.DataSize, ObjectKey: &objKey}, nil
		},
		deleteLog: func(context.Context, int64) error {
			t.Fatalf("DeleteLog should not be called on success")
			return nil
		},
	}

	bs := &stubBlobstore{
		put: func(_ context.Context, key, contentType string, payload []byte) (string, error) {
			putCalled = true
			if key != objKey {
				t.Fatalf("Put key=%q want %q", key, objKey)
			}
			if contentType != "application/gzip" {
				t.Fatalf("Put contentType=%q want application/gzip", contentType)
			}
			if string(payload) != string(data) {
				t.Fatalf("Put payload mismatch")
			}
			return "etag", nil
		},
	}

	svc := New(st, bs)
	logRow, err := svc.CreateLog(context.Background(), store.CreateLogParams{RunID: runID, ChunkNo: 1}, data)
	if err != nil {
		t.Fatalf("CreateLog error: %v", err)
	}
	if gotDataSize != int64(len(data)) {
		t.Fatalf("DataSize=%d want %d", gotDataSize, len(data))
	}
	if !putCalled {
		t.Fatalf("expected Put to be called")
	}
	if logRow.ObjectKey == nil || *logRow.ObjectKey != objKey {
		t.Fatalf("returned object_key=%v want %q", logRow.ObjectKey, objKey)
	}
}

func TestCreateLog_RollsBackMetadataOnUploadFailure(t *testing.T) {
	data := []byte("gzipped-log")
	runID := types.NewRunID()
	objKey := "logs/run/" + runID.String() + "/job/none/chunk/1/log/10.gz"

	var deletedID int64
	st := &stubStore{
		createLog: func(_ context.Context, arg store.CreateLogParams) (store.Log, error) {
			return store.Log{ID: 10, RunID: arg.RunID, JobID: arg.JobID, ChunkNo: arg.ChunkNo, DataSize: arg.DataSize, ObjectKey: &objKey}, nil
		},
		deleteLog: func(_ context.Context, id int64) error {
			deletedID = id
			return nil
		},
	}

	bs := &stubBlobstore{
		put: func(context.Context, string, string, []byte) (string, error) {
			return "", errors.New("put failed")
		},
	}

	svc := New(st, bs)
	_, err := svc.CreateLog(context.Background(), store.CreateLogParams{RunID: runID, ChunkNo: 1}, data)
	if err == nil {
		t.Fatalf("expected error")
	}
	if deletedID != 10 {
		t.Fatalf("DeleteLog id=%d want 10", deletedID)
	}
}

func TestCreateDiff_RollsBackMetadataOnUploadFailure(t *testing.T) {
	patch := []byte("gzipped-patch")
	runID := types.NewRunID()
	diffUUID := uuid.New()
	diffID := pgtype.UUID{Bytes: diffUUID, Valid: true}
	objKey := "diffs/run/" + runID.String() + "/diff/" + diffUUID.String() + ".patch.gz"

	var deleted pgtype.UUID
	st := &stubStore{
		createDiff: func(_ context.Context, arg store.CreateDiffParams) (store.Diff, error) {
			return store.Diff{ID: diffID, RunID: arg.RunID, JobID: arg.JobID, PatchSize: arg.PatchSize, ObjectKey: &objKey}, nil
		},
		deleteDiff: func(_ context.Context, id pgtype.UUID) error {
			deleted = id
			return nil
		},
	}

	bs := &stubBlobstore{
		put: func(context.Context, string, string, []byte) (string, error) {
			return "", errors.New("put failed")
		},
	}

	svc := New(st, bs)
	_, err := svc.CreateDiff(context.Background(), store.CreateDiffParams{RunID: runID}, patch)
	if err == nil {
		t.Fatalf("expected error")
	}
	if deleted != diffID {
		t.Fatalf("DeleteDiff id mismatch")
	}
}

func TestCreateArtifactBundle_RollsBackMetadataOnUploadFailure(t *testing.T) {
	bundle := []byte("gzipped-tar")
	runID := types.NewRunID()
	artifactUUID := uuid.New()
	artifactID := pgtype.UUID{Bytes: artifactUUID, Valid: true}
	objKey := "artifacts/run/" + runID.String() + "/bundle/" + artifactUUID.String() + ".tar.gz"

	var deleted pgtype.UUID
	st := &stubStore{
		createArtifactBundle: func(_ context.Context, arg store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
			return store.ArtifactBundle{ID: artifactID, RunID: arg.RunID, JobID: arg.JobID, BundleSize: arg.BundleSize, ObjectKey: &objKey}, nil
		},
		deleteArtifactBundle: func(_ context.Context, id pgtype.UUID) error {
			deleted = id
			return nil
		},
	}

	bs := &stubBlobstore{
		put: func(context.Context, string, string, []byte) (string, error) {
			return "", errors.New("put failed")
		},
	}

	svc := New(st, bs)
	_, err := svc.CreateArtifactBundle(context.Background(), store.CreateArtifactBundleParams{RunID: runID}, bundle)
	if err == nil {
		t.Fatalf("expected error")
	}
	if deleted != artifactID {
		t.Fatalf("DeleteArtifactBundle id mismatch")
	}
}

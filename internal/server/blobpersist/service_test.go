package blobpersist

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

	createLog                          func(ctx context.Context, arg store.CreateLogParams) (store.Log, error)
	deleteLog                          func(ctx context.Context, id int64) error
	createDiff                         func(ctx context.Context, arg store.CreateDiffParams) (store.Diff, error)
	deleteDiff                         func(ctx context.Context, id pgtype.UUID) error
	createArtifactBundle               func(ctx context.Context, arg store.CreateArtifactBundleParams) (store.ArtifactBundle, error)
	deleteArtifactBundle               func(ctx context.Context, id pgtype.UUID) error
	listArtifactBundlesMetaByRunAndJob func(ctx context.Context, arg store.ListArtifactBundlesMetaByRunAndJobParams) ([]store.ArtifactBundle, error)
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

func (s *stubStore) ListArtifactBundlesMetaByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesMetaByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return s.listArtifactBundlesMetaByRunAndJob(ctx, arg)
}

type stubBlobstore struct {
	put func(ctx context.Context, key, contentType string, data []byte) (string, error)
	get func(ctx context.Context, key string) (io.ReadCloser, int64, error)
}

var _ blobstore.Store = (*stubBlobstore)(nil)

func (s *stubBlobstore) Put(ctx context.Context, key, contentType string, data []byte) (string, error) {
	return s.put(ctx, key, contentType, data)
}

func (s *stubBlobstore) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if s.get == nil {
		return nil, 0, errors.New("not implemented")
	}
	return s.get(ctx, key)
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

func TestLoadRecoveryArtifact_Success(t *testing.T) {
	runID := types.NewRunID()
	jobID := types.NewJobID()
	artifactUUID := uuid.New()
	artifactID := pgtype.UUID{Bytes: artifactUUID, Valid: true}
	objectKey := "artifacts/run/" + runID.String() + "/bundle/" + artifactUUID.String() + ".tar.gz"
	candidate := []byte(`{"schema_version":1}`)
	bundle := mustTarGzBundle(t, map[string][]byte{
		"out/gate-profile-candidate.json": candidate,
	})

	st := &stubStore{
		listArtifactBundlesMetaByRunAndJob: func(_ context.Context, arg store.ListArtifactBundlesMetaByRunAndJobParams) ([]store.ArtifactBundle, error) {
			if arg.RunID != runID || arg.JobID == nil || *arg.JobID != jobID {
				t.Fatalf("unexpected list params: %+v", arg)
			}
			return []store.ArtifactBundle{{
				ID:        artifactID,
				RunID:     runID,
				JobID:     &jobID,
				ObjectKey: &objectKey,
			}}, nil
		},
	}
	bs := &stubBlobstore{
		put: func(context.Context, string, string, []byte) (string, error) { return "etag", nil },
		get: func(_ context.Context, key string) (io.ReadCloser, int64, error) {
			if key != objectKey {
				t.Fatalf("blob key=%q want %q", key, objectKey)
			}
			return io.NopCloser(bytes.NewReader(bundle)), int64(len(bundle)), nil
		},
	}

	svc := New(st, bs)
	got, err := svc.LoadRecoveryArtifact(context.Background(), runID, jobID, "/out/gate-profile-candidate.json")
	if err != nil {
		t.Fatalf("LoadRecoveryArtifact error: %v", err)
	}
	if string(got) != string(candidate) {
		t.Fatalf("candidate mismatch: got=%q want=%q", string(got), string(candidate))
	}
}

func TestLoadRecoveryArtifact_NotFound(t *testing.T) {
	runID := types.NewRunID()
	jobID := types.NewJobID()
	artifactUUID := uuid.New()
	artifactID := pgtype.UUID{Bytes: artifactUUID, Valid: true}
	objectKey := "artifacts/run/" + runID.String() + "/bundle/" + artifactUUID.String() + ".tar.gz"
	bundle := mustTarGzBundle(t, map[string][]byte{
		"out/something-else.json": []byte(`{"ok":true}`),
	})

	st := &stubStore{
		listArtifactBundlesMetaByRunAndJob: func(_ context.Context, _ store.ListArtifactBundlesMetaByRunAndJobParams) ([]store.ArtifactBundle, error) {
			return []store.ArtifactBundle{{ID: artifactID, RunID: runID, JobID: &jobID, ObjectKey: &objectKey}}, nil
		},
	}
	bs := &stubBlobstore{
		put: func(context.Context, string, string, []byte) (string, error) { return "etag", nil },
		get: func(_ context.Context, _ string) (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewReader(bundle)), int64(len(bundle)), nil
		},
	}

	svc := New(st, bs)
	_, err := svc.LoadRecoveryArtifact(context.Background(), runID, jobID, "/out/gate-profile-candidate.json")
	if !errors.Is(err, ErrRecoveryArtifactNotFound) {
		t.Fatalf("expected ErrRecoveryArtifactNotFound, got %v", err)
	}
}

func TestLoadRecoveryArtifact_Unreadable(t *testing.T) {
	runID := types.NewRunID()
	jobID := types.NewJobID()
	artifactUUID := uuid.New()
	artifactID := pgtype.UUID{Bytes: artifactUUID, Valid: true}
	objectKey := "artifacts/run/" + runID.String() + "/bundle/" + artifactUUID.String() + ".tar.gz"

	st := &stubStore{
		listArtifactBundlesMetaByRunAndJob: func(_ context.Context, _ store.ListArtifactBundlesMetaByRunAndJobParams) ([]store.ArtifactBundle, error) {
			return []store.ArtifactBundle{{ID: artifactID, RunID: runID, JobID: &jobID, ObjectKey: &objectKey}}, nil
		},
	}
	bs := &stubBlobstore{
		put: func(context.Context, string, string, []byte) (string, error) { return "etag", nil },
		get: func(_ context.Context, _ string) (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewReader([]byte("not-gzip"))), int64(len("not-gzip")), nil
		},
	}

	svc := New(st, bs)
	_, err := svc.LoadRecoveryArtifact(context.Background(), runID, jobID, "/out/gate-profile-candidate.json")
	if !errors.Is(err, ErrRecoveryArtifactUnreadable) {
		t.Fatalf("expected ErrRecoveryArtifactUnreadable, got %v", err)
	}
}

func TestLoadRecoveryArtifact_InvalidJSON(t *testing.T) {
	runID := types.NewRunID()
	jobID := types.NewJobID()
	artifactUUID := uuid.New()
	artifactID := pgtype.UUID{Bytes: artifactUUID, Valid: true}
	objectKey := "artifacts/run/" + runID.String() + "/bundle/" + artifactUUID.String() + ".tar.gz"
	bundle := mustTarGzBundle(t, map[string][]byte{
		"out/gate-profile-candidate.json": []byte("not-json"),
	})

	st := &stubStore{
		listArtifactBundlesMetaByRunAndJob: func(_ context.Context, _ store.ListArtifactBundlesMetaByRunAndJobParams) ([]store.ArtifactBundle, error) {
			return []store.ArtifactBundle{{ID: artifactID, RunID: runID, JobID: &jobID, ObjectKey: &objectKey}}, nil
		},
	}
	bs := &stubBlobstore{
		put: func(context.Context, string, string, []byte) (string, error) { return "etag", nil },
		get: func(_ context.Context, _ string) (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewReader(bundle)), int64(len(bundle)), nil
		},
	}

	svc := New(st, bs)
	_, err := svc.LoadRecoveryArtifact(context.Background(), runID, jobID, "/out/gate-profile-candidate.json")
	if !errors.Is(err, ErrRecoveryArtifactInvalidJSON) {
		t.Fatalf("expected ErrRecoveryArtifactInvalidJSON, got %v", err)
	}
}

func mustTarGzBundle(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %q: %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("write payload %q: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return b.Bytes()
}

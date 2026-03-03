package handlers

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type stubGateProfileResolverStore struct {
	stackByImage map[string]int64
	anyStackID   int64
	anyStackErr  error

	exactRow gateProfileRow
	exactErr error

	latestRow gateProfileRow
	latestErr error

	defaultRow gateProfileRow
	defaultErr error

	upsertRow        gateProfileRow
	upsertErr        error
	upsertCalled     bool
	upsertRepoID     types.RepoID
	upsertRepoSHA    string
	upsertStackID    int64
	upsertObjectKey  string
	linkErr          error
	linkCalled       bool
	linkJobID        types.JobID
	linkProfileID    int64
	resolveImageCall string
}

func (s *stubGateProfileResolverStore) ResolveStackIDByImage(_ context.Context, image string) (int64, error) {
	s.resolveImageCall = image
	if stackID, ok := s.stackByImage[image]; ok {
		return stackID, nil
	}
	return 0, pgx.ErrNoRows
}

func (s *stubGateProfileResolverStore) ResolveAnyStackID(_ context.Context) (int64, error) {
	if s.anyStackErr != nil {
		return 0, s.anyStackErr
	}
	if s.anyStackID == 0 {
		return 0, pgx.ErrNoRows
	}
	return s.anyStackID, nil
}

func (s *stubGateProfileResolverStore) GetExactGateProfile(_ context.Context, _ types.RepoID, _ string, _ int64) (gateProfileRow, error) {
	if s.exactErr != nil {
		return gateProfileRow{}, s.exactErr
	}
	return s.exactRow, nil
}

func (s *stubGateProfileResolverStore) GetLatestRepoGateProfile(_ context.Context, _ types.RepoID, _ int64) (gateProfileRow, error) {
	if s.latestErr != nil {
		return gateProfileRow{}, s.latestErr
	}
	return s.latestRow, nil
}

func (s *stubGateProfileResolverStore) GetDefaultGateProfile(_ context.Context, _ int64) (gateProfileRow, error) {
	if s.defaultErr != nil {
		return gateProfileRow{}, s.defaultErr
	}
	return s.defaultRow, nil
}

func (s *stubGateProfileResolverStore) UpsertExactGateProfile(_ context.Context, repoID types.RepoID, repoSHA string, stackID int64, objectKey string) (gateProfileRow, error) {
	s.upsertCalled = true
	s.upsertRepoID = repoID
	s.upsertRepoSHA = repoSHA
	s.upsertStackID = stackID
	s.upsertObjectKey = objectKey
	if s.upsertErr != nil {
		return gateProfileRow{}, s.upsertErr
	}
	return s.upsertRow, nil
}

func (s *stubGateProfileResolverStore) UpsertGateJobProfileLink(_ context.Context, jobID types.JobID, profileID int64) error {
	s.linkCalled = true
	s.linkJobID = jobID
	s.linkProfileID = profileID
	return s.linkErr
}

type stubBlobStore struct {
	getPayloadByKey map[string][]byte
	putCalled       bool
	putKey          string
	putBody         []byte
}

func (s *stubBlobStore) Put(_ context.Context, key, _ string, data []byte) (string, error) {
	s.putCalled = true
	s.putKey = key
	s.putBody = append([]byte(nil), data...)
	if s.getPayloadByKey == nil {
		s.getPayloadByKey = map[string][]byte{}
	}
	s.getPayloadByKey[key] = append([]byte(nil), data...)
	return "etag", nil
}

func (s *stubBlobStore) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	body, ok := s.getPayloadByKey[key]
	if !ok {
		return nil, 0, pgx.ErrNoRows
	}
	return io.NopCloser(bytes.NewReader(body)), int64(len(body)), nil
}

func (s *stubBlobStore) Delete(_ context.Context, _ string) error {
	return nil
}

func TestGateProfileResolver_ExactHit(t *testing.T) {
	t.Parallel()

	repoID := types.NewRepoID()
	const shaIn = "0123456789abcdef0123456789abcdef01234567"
	exactKey := "gate-profiles/exact.json"
	exactPayload := []byte(`{"schema_version":1}`)

	st := &stubGateProfileResolverStore{
		stackByImage: map[string]int64{"docker.io/stack:latest": 7},
		exactRow: gateProfileRow{
			ID:        11,
			RepoID:    repoID,
			RepoSHA:   shaIn,
			RepoSHA8:  shaIn[:8],
			StackID:   7,
			ObjectKey: exactKey,
		},
	}
	bs := &stubBlobStore{
		getPayloadByKey: map[string][]byte{
			exactKey: exactPayload,
		},
	}
	resolver := &dbGateProfileResolver{st: st, bs: bs}

	job := store.Job{RepoID: repoID, RepoShaIn: shaIn, JobImage: "docker.io/stack:latest"}
	resolution, err := resolver.ResolveGateProfileForJob(context.Background(), job)
	if err != nil {
		t.Fatalf("ResolveGateProfileForJob() error = %v", err)
	}
	if resolution == nil {
		t.Fatal("expected non-nil gate profile resolution")
	}
	profileID := resolution.ProfileID
	profilePayload := resolution.Payload
	if profileID != 11 {
		t.Fatalf("profile_id = %d, want 11", profileID)
	}
	if string(profilePayload) != string(exactPayload) {
		t.Fatalf("profile payload mismatch: got %q want %q", profilePayload, exactPayload)
	}
	if bs.putCalled {
		t.Fatal("did not expect blob copy on exact hit")
	}
	if st.upsertCalled {
		t.Fatal("did not expect exact upsert on exact hit")
	}
	if !st.linkCalled {
		t.Fatal("expected gate link upsert on exact hit")
	}
	if st.linkJobID != job.ID || st.linkProfileID != 11 {
		t.Fatalf("unexpected gate link args: job=%s profile=%d", st.linkJobID, st.linkProfileID)
	}
	if !resolution.ExactHit {
		t.Fatal("ExactHit=false, want true")
	}
}

func TestGateProfileResolver_FallbackRepoStackCopiesAndUpsertsExact(t *testing.T) {
	t.Parallel()

	repoID := types.NewRepoID()
	const shaIn = "0123456789abcdef0123456789abcdef01234567"
	fallbackKey := "gate-profiles/repo-latest.json"
	fallbackPayload := []byte(`{"schema_version":1,"runner_mode":"simple"}`)

	st := &stubGateProfileResolverStore{
		stackByImage: map[string]int64{"docker.io/stack:latest": 3},
		exactErr:     pgx.ErrNoRows,
		latestRow: gateProfileRow{
			ID:        21,
			RepoID:    repoID,
			StackID:   3,
			ObjectKey: fallbackKey,
		},
		upsertRow: gateProfileRow{
			ID:        22,
			RepoID:    repoID,
			RepoSHA:   shaIn,
			RepoSHA8:  shaIn[:8],
			StackID:   3,
			ObjectKey: "gate-profiles/copied.json",
		},
	}
	bs := &stubBlobStore{
		getPayloadByKey: map[string][]byte{
			fallbackKey: fallbackPayload,
		},
	}
	resolver := &dbGateProfileResolver{st: st, bs: bs}

	job := store.Job{RepoID: repoID, RepoShaIn: shaIn, JobImage: "docker.io/stack:latest"}
	resolution, err := resolver.ResolveGateProfileForJob(context.Background(), job)
	if err != nil {
		t.Fatalf("ResolveGateProfileForJob() error = %v", err)
	}
	if resolution == nil {
		t.Fatal("expected non-nil gate profile resolution")
	}
	profileID := resolution.ProfileID
	profilePayload := resolution.Payload
	if profileID != 22 {
		t.Fatalf("profile_id = %d, want 22", profileID)
	}
	if string(profilePayload) != string(fallbackPayload) {
		t.Fatalf("profile payload mismatch: got %q want %q", profilePayload, fallbackPayload)
	}
	if !bs.putCalled {
		t.Fatal("expected fallback blob copy")
	}
	if !st.upsertCalled {
		t.Fatal("expected exact upsert after fallback copy")
	}
	if st.upsertRepoID != repoID || st.upsertRepoSHA != shaIn || st.upsertStackID != 3 {
		t.Fatalf("unexpected upsert args: repo=%s sha=%s stack=%d", st.upsertRepoID, st.upsertRepoSHA, st.upsertStackID)
	}
	if st.upsertObjectKey == "" || !strings.HasPrefix(st.upsertObjectKey, "gate-profiles/repos/") {
		t.Fatalf("unexpected upsert object key %q", st.upsertObjectKey)
	}
	if !st.linkCalled {
		t.Fatal("expected gate link upsert after fallback upsert")
	}
	if st.linkJobID != job.ID || st.linkProfileID != 22 {
		t.Fatalf("unexpected gate link args: job=%s profile=%d", st.linkJobID, st.linkProfileID)
	}
	if resolution.ExactHit {
		t.Fatal("ExactHit=true, want false on fallback")
	}
}

func TestGateProfileResolver_FallbackDefaultStack(t *testing.T) {
	t.Parallel()

	repoID := types.NewRepoID()
	const shaIn = "fedcba9876543210fedcba9876543210fedcba98"
	defaultKey := "gate-profiles/default.json"
	defaultPayload := []byte(`{"schema_version":1,"targets":{"active":"unit"}}`)

	st := &stubGateProfileResolverStore{
		stackByImage: map[string]int64{},
		anyStackID:   9,
		exactErr:     pgx.ErrNoRows,
		latestErr:    pgx.ErrNoRows,
		defaultRow: gateProfileRow{
			ID:        31,
			StackID:   9,
			ObjectKey: defaultKey,
		},
		upsertRow: gateProfileRow{
			ID:        32,
			RepoID:    repoID,
			RepoSHA:   shaIn,
			RepoSHA8:  shaIn[:8],
			StackID:   9,
			ObjectKey: "gate-profiles/default-copied.json",
		},
	}
	bs := &stubBlobStore{
		getPayloadByKey: map[string][]byte{
			defaultKey: defaultPayload,
		},
	}
	resolver := &dbGateProfileResolver{st: st, bs: bs}

	job := store.Job{RepoID: repoID, RepoShaIn: shaIn}
	resolution, err := resolver.ResolveGateProfileForJob(context.Background(), job)
	if err != nil {
		t.Fatalf("ResolveGateProfileForJob() error = %v", err)
	}
	if resolution == nil {
		t.Fatal("expected non-nil gate profile resolution")
	}
	profileID := resolution.ProfileID
	profilePayload := resolution.Payload
	if profileID != 32 {
		t.Fatalf("profile_id = %d, want 32", profileID)
	}
	if string(profilePayload) != string(defaultPayload) {
		t.Fatalf("profile payload mismatch: got %q want %q", profilePayload, defaultPayload)
	}
	if !bs.putCalled {
		t.Fatal("expected default fallback blob copy")
	}
	if !st.upsertCalled {
		t.Fatal("expected exact upsert after default fallback copy")
	}
	if !st.linkCalled {
		t.Fatal("expected gate link upsert after default fallback upsert")
	}
	if st.linkJobID != job.ID || st.linkProfileID != 32 {
		t.Fatalf("unexpected gate link args: job=%s profile=%d", st.linkJobID, st.linkProfileID)
	}
	if resolution.ExactHit {
		t.Fatal("ExactHit=true, want false on default fallback")
	}
}

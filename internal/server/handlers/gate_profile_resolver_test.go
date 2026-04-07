package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type stubGateProfileResolverStore struct {
	stackByImage    map[string]int64
	stackRowByImage map[string]gateProfileStackRow
	stackByRequired map[string]int64
	anyStackID      int64
	anyStackErr     error
	repoSHAStackID  int64
	repoSHAErr      error

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
	resolveRepoCall  bool
	resolveAnyCall   bool
	resolveExactCall bool
	resolveRequired  *GateProfileLookupStack
}

func (s *stubGateProfileResolverStore) ResolveStackIDByImage(_ context.Context, image string) (int64, error) {
	s.resolveImageCall = image
	if stackID, ok := s.stackByImage[image]; ok {
		return stackID, nil
	}
	return 0, pgx.ErrNoRows
}

func (s *stubGateProfileResolverStore) ResolveStackRowByImage(_ context.Context, image string) (gateProfileStackRow, error) {
	s.resolveImageCall = image
	if row, ok := s.stackRowByImage[image]; ok {
		return row, nil
	}
	if stackID, ok := s.stackByImage[image]; ok {
		return gateProfileStackRow{ID: stackID}, nil
	}
	return gateProfileStackRow{}, pgx.ErrNoRows
}

func strictStackKey(language, tool, release string) string {
	return strings.TrimSpace(language) + "|" + strings.TrimSpace(tool) + "|" + strings.TrimSpace(release)
}

func (s *stubGateProfileResolverStore) ResolveStackIDByRequiredStack(_ context.Context, language, tool, release string) (int64, error) {
	s.resolveRequired = &GateProfileLookupStack{
		Language: language,
		Tool:     tool,
		Release:  release,
	}
	if stackID, ok := s.stackByRequired[strictStackKey(language, tool, release)]; ok {
		return stackID, nil
	}
	return 0, pgx.ErrNoRows
}

func (s *stubGateProfileResolverStore) ResolveStackIDByRepoSHA(_ context.Context, _ types.RepoID, _ string) (int64, error) {
	s.resolveRepoCall = true
	if s.repoSHAErr != nil {
		return 0, s.repoSHAErr
	}
	if s.repoSHAStackID != 0 {
		return s.repoSHAStackID, nil
	}
	return 0, pgx.ErrNoRows
}

func (s *stubGateProfileResolverStore) ResolveAnyStackID(_ context.Context) (int64, error) {
	s.resolveAnyCall = true
	if s.anyStackErr != nil {
		return 0, s.anyStackErr
	}
	if s.anyStackID == 0 {
		return 0, pgx.ErrNoRows
	}
	return s.anyStackID, nil
}

func (s *stubGateProfileResolverStore) GetExactGateProfile(_ context.Context, _ types.RepoID, _ string, _ int64) (gateProfileRow, error) {
	s.resolveExactCall = true
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

type gateProfileResolverCase struct {
	name        string
	st          *stubGateProfileResolverStore
	bs          *stubBlobStore
	job         store.Job
	constraints GateProfileLookupConstraints

	wantNil       bool
	wantProfileID int64
	wantPayload   []byte
	wantExactHit  bool

	wantUpsert  bool
	wantLink    bool
	wantBlobPut bool

	wantResolveImage    bool
	wantResolveRepo     bool
	wantResolveAny      bool
	wantResolveExact    bool
	wantResolveRequired bool

	wantUpsertRepoID          *types.RepoID
	wantUpsertStackID         *int64
	wantUpsertObjectKeyPrefix string
	wantResolvedRequiredTool  string
}

func assertGateProfileResolution(t *testing.T, tc gateProfileResolverCase, resolution *GateProfileResolution, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("ResolveGateProfileForJob() error = %v", err)
	}
	if tc.wantNil {
		if resolution != nil {
			t.Fatalf("expected nil resolution, got %+v", resolution)
		}
	} else {
		if resolution == nil {
			t.Fatal("expected non-nil resolution")
		}
		if resolution.ProfileID != tc.wantProfileID {
			t.Fatalf("ProfileID = %d, want %d", resolution.ProfileID, tc.wantProfileID)
		}
		if tc.wantPayload != nil && string(resolution.Payload) != string(tc.wantPayload) {
			t.Fatalf("Payload = %q, want %q", resolution.Payload, tc.wantPayload)
		}
		if resolution.ExactHit != tc.wantExactHit {
			t.Fatalf("ExactHit = %v, want %v", resolution.ExactHit, tc.wantExactHit)
		}
	}
	if tc.st.upsertCalled != tc.wantUpsert {
		t.Fatalf("upsertCalled = %v, want %v", tc.st.upsertCalled, tc.wantUpsert)
	}
	if tc.st.linkCalled != tc.wantLink {
		t.Fatalf("linkCalled = %v, want %v", tc.st.linkCalled, tc.wantLink)
	}
	if tc.bs.putCalled != tc.wantBlobPut {
		t.Fatalf("putCalled = %v, want %v", tc.bs.putCalled, tc.wantBlobPut)
	}
	if (tc.st.resolveImageCall != "") != tc.wantResolveImage {
		t.Fatalf("resolveImageCall = %q, wantResolveImage = %v", tc.st.resolveImageCall, tc.wantResolveImage)
	}
	if tc.st.resolveRepoCall != tc.wantResolveRepo {
		t.Fatalf("resolveRepoCall = %v, want %v", tc.st.resolveRepoCall, tc.wantResolveRepo)
	}
	if tc.st.resolveAnyCall != tc.wantResolveAny {
		t.Fatalf("resolveAnyCall = %v, want %v", tc.st.resolveAnyCall, tc.wantResolveAny)
	}
	if tc.st.resolveExactCall != tc.wantResolveExact {
		t.Fatalf("resolveExactCall = %v, want %v", tc.st.resolveExactCall, tc.wantResolveExact)
	}
	if (tc.st.resolveRequired != nil) != tc.wantResolveRequired {
		t.Fatalf("resolveRequired = %v, wantResolveRequired = %v", tc.st.resolveRequired, tc.wantResolveRequired)
	}
	if tc.wantUpsertRepoID != nil && tc.st.upsertRepoID != *tc.wantUpsertRepoID {
		t.Fatalf("upsertRepoID = %s, want %s", tc.st.upsertRepoID, *tc.wantUpsertRepoID)
	}
	if tc.wantUpsertStackID != nil && tc.st.upsertStackID != *tc.wantUpsertStackID {
		t.Fatalf("upsertStackID = %d, want %d", tc.st.upsertStackID, *tc.wantUpsertStackID)
	}
	if tc.wantUpsertObjectKeyPrefix != "" && !strings.HasPrefix(tc.st.upsertObjectKey, tc.wantUpsertObjectKeyPrefix) {
		t.Fatalf("upsertObjectKey = %q, want prefix %q", tc.st.upsertObjectKey, tc.wantUpsertObjectKeyPrefix)
	}
	if tc.wantResolvedRequiredTool != "" {
		if tc.st.resolveRequired == nil {
			t.Fatal("expected resolveRequired to be set for tool assertion")
		}
		if tc.st.resolveRequired.Tool != tc.wantResolvedRequiredTool {
			t.Fatalf("resolveRequired.Tool = %q, want %q", tc.st.resolveRequired.Tool, tc.wantResolvedRequiredTool)
		}
	}
}

func TestGateProfileResolver_NormalResolution(t *testing.T) {
	t.Parallel()

	repoA := types.NewRepoID()
	repoB := types.NewRepoID()
	repoC := types.NewRepoID()
	repoD := types.NewRepoID()

	const sha1 = "0123456789abcdef0123456789abcdef01234567"
	const sha2 = "fedcba9876543210fedcba9876543210fedcba98"

	stackID3 := int64(3)
	exactPayload := []byte(`{"schema_version":1}`)
	latestPayload := []byte(`{"schema_version":1,"runner_mode":"simple"}`)
	defaultPayload := []byte(`{"schema_version":1,"targets":{"active":"unit"}}`)

	cases := []gateProfileResolverCase{
		{
			name: "exact_hit",
			st: &stubGateProfileResolverStore{
				stackByImage: map[string]int64{"docker.io/stack:latest": 7},
				exactRow: gateProfileRow{
					ID: 11, RepoID: repoA, RepoSHA: sha1, RepoSHA8: sha1[:8],
					StackID: 7, ObjectKey: "gate-profiles/exact.json",
				},
			},
			bs: &stubBlobStore{getPayloadByKey: map[string][]byte{
				"gate-profiles/exact.json": exactPayload,
			}},
			job:              store.Job{RepoID: repoA, RepoShaIn: sha1, JobImage: "docker.io/stack:latest"},
			wantProfileID:    11,
			wantPayload:      exactPayload,
			wantExactHit:     true,
			wantLink:         true,
			wantResolveExact: true,
			wantResolveImage: true,
		},
		{
			name: "fallback_repo_latest",
			st: &stubGateProfileResolverStore{
				stackByImage: map[string]int64{"docker.io/stack:latest": 3},
				exactErr:     pgx.ErrNoRows,
				latestRow: gateProfileRow{
					ID: 21, RepoID: repoB, StackID: 3,
					ObjectKey: "gate-profiles/repo-latest.json",
				},
				upsertRow: gateProfileRow{
					ID: 22, RepoID: repoB, RepoSHA: sha1, RepoSHA8: sha1[:8],
					StackID: 3, ObjectKey: "gate-profiles/copied.json",
				},
			},
			bs: &stubBlobStore{getPayloadByKey: map[string][]byte{
				"gate-profiles/repo-latest.json": latestPayload,
			}},
			job:                       store.Job{RepoID: repoB, RepoShaIn: sha1, JobImage: "docker.io/stack:latest"},
			wantProfileID:             22,
			wantPayload:               latestPayload,
			wantUpsert:                true,
			wantBlobPut:               true,
			wantLink:                  true,
			wantResolveExact:          true,
			wantResolveImage:          true,
			wantUpsertRepoID:          &repoB,
			wantUpsertStackID:         &stackID3,
			wantUpsertObjectKeyPrefix: "gate-profiles/repos/",
		},
		{
			name: "fallback_default",
			st: &stubGateProfileResolverStore{
				stackByImage: map[string]int64{},
				repoSHAStackID: 9,
				exactErr:     pgx.ErrNoRows,
				latestErr:    pgx.ErrNoRows,
				defaultRow: gateProfileRow{
					ID: 31, StackID: 9, ObjectKey: "gate-profiles/default.json",
				},
				upsertRow: gateProfileRow{
					ID: 32, RepoID: repoC, RepoSHA: sha2, RepoSHA8: sha2[:8],
					StackID: 9, ObjectKey: "gate-profiles/default-copied.json",
				},
			},
			bs: &stubBlobStore{getPayloadByKey: map[string][]byte{
				"gate-profiles/default.json": defaultPayload,
			}},
			job:              store.Job{RepoID: repoC, RepoShaIn: sha2},
			wantProfileID:    32,
			wantPayload:      defaultPayload,
			wantUpsert:       true,
			wantBlobPut:      true,
			wantLink:         true,
			wantResolveExact: true,
			wantResolveRepo:  true,
		},
		{
			name: "no_stack_signal_returns_nil",
			st: &stubGateProfileResolverStore{
				stackByImage: map[string]int64{},
				exactErr:     pgx.ErrNoRows,
				latestErr:    pgx.ErrNoRows,
				defaultErr:   pgx.ErrNoRows,
			},
			bs:              &stubBlobStore{},
			job:             store.Job{RepoID: repoC, RepoShaIn: sha2},
			wantNil:         true,
			wantResolveRepo: true,
		},
		{
			name: "exact_hit_short_circuits_errors",
			st: &stubGateProfileResolverStore{
				stackByImage: map[string]int64{"docker.io/stack:latest": 7},
				exactRow: gateProfileRow{
					ID: 11, RepoID: repoA, RepoSHA: sha1, RepoSHA8: sha1[:8],
					StackID: 7, ObjectKey: "gate-profiles/exact.json",
				},
				latestErr:  fmt.Errorf("db connection reset"),
				defaultErr: fmt.Errorf("db connection reset"),
			},
			bs: &stubBlobStore{getPayloadByKey: map[string][]byte{
				"gate-profiles/exact.json": exactPayload,
			}},
			job:              store.Job{RepoID: repoA, RepoShaIn: sha1, JobImage: "docker.io/stack:latest"},
			wantProfileID:    11,
			wantPayload:      exactPayload,
			wantExactHit:     true,
			wantLink:         true,
			wantResolveExact: true,
			wantResolveImage: true,
		},
		{
			name: "latest_hit_short_circuits_errors",
			st: &stubGateProfileResolverStore{
				stackByImage: map[string]int64{"docker.io/stack:latest": 5},
				exactErr:     pgx.ErrNoRows,
				latestRow: gateProfileRow{
					ID: 41, RepoID: repoD, StackID: 5,
					ObjectKey: "gate-profiles/latest.json",
				},
				defaultErr: fmt.Errorf("db connection reset"),
				upsertRow: gateProfileRow{
					ID: 42, RepoID: repoD, RepoSHA: sha1, RepoSHA8: sha1[:8],
					StackID: 5, ObjectKey: "gate-profiles/promoted.json",
				},
			},
			bs: &stubBlobStore{getPayloadByKey: map[string][]byte{
				"gate-profiles/latest.json": latestPayload,
			}},
			job:              store.Job{RepoID: repoD, RepoShaIn: sha1, JobImage: "docker.io/stack:latest"},
			wantProfileID:    42,
			wantPayload:      latestPayload,
			wantUpsert:       true,
			wantBlobPut:      true,
			wantLink:         true,
			wantResolveExact: true,
			wantResolveImage: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resolver := &dbGateProfileResolver{st: tc.st, bs: tc.bs}
			resolution, err := resolver.ResolveGateProfileForJob(context.Background(), tc.job, tc.constraints)
			assertGateProfileResolution(t, tc, resolution, err)
		})
	}
}

func TestGateProfileResolver_StrictStack(t *testing.T) {
	t.Parallel()

	repoA := types.NewRepoID()
	repoB := types.NewRepoID()
	repoC := types.NewRepoID()
	repoD := types.NewRepoID()

	const sha1 = "1234567890abcdef1234567890abcdef12345678"
	const sha2 = "89abcdef0123456789abcdef0123456789abcdef"
	const sha3 = "fedcba9876543210fedcba9876543210fedcba98"
	const sha4 = "0123456789abcdef0123456789abcdef01234567"

	exactPayload := []byte(`{"schema_version":1}`)

	cases := []gateProfileResolverCase{
		{
			name: "required_lookup_exact_hit",
			st: &stubGateProfileResolverStore{
				stackByRequired: map[string]int64{strictStackKey("java", "maven", "17"): 11},
				exactRow: gateProfileRow{
					ID: 81, RepoID: repoA, RepoSHA: sha1, RepoSHA8: sha1[:8],
					StackID: 11, ObjectKey: "gate-profiles/strict.json",
				},
			},
			bs: &stubBlobStore{getPayloadByKey: map[string][]byte{
				"gate-profiles/strict.json": exactPayload,
			}},
			job: store.Job{RepoID: repoA, RepoShaIn: sha1},
			constraints: GateProfileLookupConstraints{
				StrictStack: &GateProfileLookupStack{Language: "java", Tool: "maven", Release: "17"},
			},
			wantProfileID:       81,
			wantPayload:         exactPayload,
			wantExactHit:        true,
			wantLink:            true,
			wantResolveRequired: true,
			wantResolveExact:    true,
		},
		{
			name: "no_match_skips_all",
			st: &stubGateProfileResolverStore{
				repoSHAStackID: 7,
				anyStackID:     9,
			},
			bs:  &stubBlobStore{},
			job: store.Job{RepoID: repoB, RepoShaIn: sha2, JobImage: "docker.io/stack:latest"},
			constraints: GateProfileLookupConstraints{
				StrictStack: &GateProfileLookupStack{Language: "java", Tool: "gradle", Release: "17"},
			},
			wantNil:             true,
			wantResolveRequired: true,
			wantResolveImage:    true,
		},
		{
			name: "toolless_fills_from_image",
			st: &stubGateProfileResolverStore{
				stackRowByImage: map[string]gateProfileStackRow{
					"ghcr.io/iw2rmb/ploy/gate-gradle:jdk11": {
						ID: 3, Lang: "java", Tool: "gradle", Release: "11",
					},
				},
				stackByRequired: map[string]int64{
					strictStackKey("java", "gradle", "11"): 3,
				},
				exactRow: gateProfileRow{
					ID: 91, RepoID: repoC, RepoSHA: sha3, RepoSHA8: sha3[:8],
					StackID: 3, ObjectKey: "gate-profiles/strict-toolless.json",
				},
			},
			bs: &stubBlobStore{getPayloadByKey: map[string][]byte{
				"gate-profiles/strict-toolless.json": exactPayload,
			}},
			job: store.Job{RepoID: repoC, RepoShaIn: sha3, JobImage: "ghcr.io/iw2rmb/ploy/gate-gradle:jdk11"},
			constraints: GateProfileLookupConstraints{
				StrictStack: &GateProfileLookupStack{Language: "java", Release: "11"},
			},
			wantProfileID:            91,
			wantPayload:              exactPayload,
			wantExactHit:             true,
			wantLink:                 true,
			wantResolveRequired:      true,
			wantResolveImage:         true,
			wantResolveExact:         true,
			wantResolvedRequiredTool: "gradle",
		},
		{
			name: "toolless_image_mismatch_skips",
			st: &stubGateProfileResolverStore{
				stackRowByImage: map[string]gateProfileStackRow{
					"ghcr.io/iw2rmb/ploy/gate-gradle:jdk17": {
						ID: 4, Lang: "java", Tool: "gradle", Release: "17",
					},
				},
				repoSHAStackID: 7,
				anyStackID:     9,
			},
			bs:  &stubBlobStore{},
			job: store.Job{RepoID: repoD, RepoShaIn: sha4, JobImage: "ghcr.io/iw2rmb/ploy/gate-gradle:jdk17"},
			constraints: GateProfileLookupConstraints{
				StrictStack: &GateProfileLookupStack{Language: "java", Release: "11"},
			},
			wantNil:          true,
			wantResolveImage: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resolver := &dbGateProfileResolver{st: tc.st, bs: tc.bs}
			resolution, err := resolver.ResolveGateProfileForJob(context.Background(), tc.job, tc.constraints)
			assertGateProfileResolution(t, tc, resolution, err)
		})
	}
}

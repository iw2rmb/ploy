package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestResolveUpstreamSBOMInputHash_ReturnsDigestHexForSBOMPredecessor(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	currentID := domaintypes.NewJobID()
	predecessorID := domaintypes.NewJobID()
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	name := "mig-out"

	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:      predecessorID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			NextID:  &currentID,
			JobType: domaintypes.JobTypeSBOM,
			Status:  domaintypes.JobStatusSuccess,
		},
		{
			ID:      currentID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			JobType: domaintypes.JobTypePreGate,
		},
	}
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
		{Name: &name, Digest: &digest},
	}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	hash, required, available, err := svc.resolveUpstreamSBOMInputHash(context.Background(), store.Job{
		ID:      currentID,
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
	})
	if err != nil {
		t.Fatalf("resolveUpstreamSBOMInputHash() error = %v", err)
	}
	if !required || !available {
		t.Fatalf("required=%v available=%v, want true/true", required, available)
	}
	want := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if hash != want {
		t.Fatalf("hash = %q, want %q", hash, want)
	}
}

func TestResolveUpstreamSBOMBundleForJob_ReturnsDigestAndArtifactID(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	currentID := domaintypes.NewJobID()
	predecessorID := domaintypes.NewJobID()
	artifactID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	name := "mig-out"

	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:      predecessorID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			NextID:  &currentID,
			JobType: domaintypes.JobTypeSBOM,
			Status:  domaintypes.JobStatusSuccess,
		},
		{
			ID:      currentID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			JobType: domaintypes.JobTypeHook,
		},
	}
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
		{
			ID:     pgtype.UUID{Bytes: artifactID, Valid: true},
			Name:   &name,
			Digest: &digest,
		},
	}

	resolved, required, available, err := resolveUpstreamSBOMBundleForJob(context.Background(), st, store.Job{
		ID:      currentID,
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
	})
	if err != nil {
		t.Fatalf("resolveUpstreamSBOMBundleForJob() error = %v", err)
	}
	if !required || !available {
		t.Fatalf("required=%v available=%v, want true/true", required, available)
	}
	wantDigest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if resolved.Digest != wantDigest {
		t.Fatalf("digest = %q, want %q", resolved.Digest, wantDigest)
	}
	if resolved.ArtifactID != artifactID.String() {
		t.Fatalf("artifact id = %q, want %q", resolved.ArtifactID, artifactID.String())
	}
}

func TestResolveUpstreamSBOMInputHash_InvalidDigestMakesReplayIneligible(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	currentID := domaintypes.NewJobID()
	predecessorID := domaintypes.NewJobID()
	name := "mig-out"
	invalid := "sha256:not-a-hex-digest"

	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:      predecessorID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			NextID:  &currentID,
			JobType: domaintypes.JobTypeSBOM,
			Status:  domaintypes.JobStatusSuccess,
		},
		{
			ID:      currentID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			JobType: domaintypes.JobTypePreGate,
		},
	}
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
		{Name: &name, Digest: &invalid},
	}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	hash, required, available, err := svc.resolveUpstreamSBOMInputHash(context.Background(), store.Job{
		ID:      currentID,
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
	})
	if err != nil {
		t.Fatalf("resolveUpstreamSBOMInputHash() error = %v", err)
	}
	if !required || available {
		t.Fatalf("required=%v available=%v, want true/false", required, available)
	}
	if hash != "" {
		t.Fatalf("hash = %q, want empty", hash)
	}
}

func TestResolveRuntimeInputHash_RecoveryContextAffectsHash(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	jobID := domaintypes.NewJobID()
	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      jobID,
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
		JobType: domaintypes.JobTypeHeal,
	}}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	job := store.Job{ID: jobID, RunID: runID, RepoID: repoID, Attempt: 1, JobType: domaintypes.JobTypeHeal}
	hashA, okA, err := svc.resolveRuntimeInputHash(context.Background(), job, claimResponsePayload{
		RecoveryContext: &contracts.RecoveryClaimContext{BuildGateLog: "A"},
	})
	if err != nil {
		t.Fatalf("resolveRuntimeInputHash(A) error = %v", err)
	}
	hashB, okB, err := svc.resolveRuntimeInputHash(context.Background(), job, claimResponsePayload{
		RecoveryContext: &contracts.RecoveryClaimContext{BuildGateLog: "B"},
	})
	if err != nil {
		t.Fatalf("resolveRuntimeInputHash(B) error = %v", err)
	}
	if !okA || !okB {
		t.Fatalf("expected eligible hashes, got okA=%v okB=%v", okA, okB)
	}
	if hashA == hashB {
		t.Fatal("runtime input hash must differ for different recovery context")
	}
}

func TestResolveRuntimeInputHash_HookRuntimeAffectsHash(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	jobID := domaintypes.NewJobID()
	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      jobID,
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
		JobType: domaintypes.JobTypeHook,
	}}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	job := store.Job{ID: jobID, RunID: runID, RepoID: repoID, Attempt: 1, JobType: domaintypes.JobTypeHook}
	hashA, okA, err := svc.resolveRuntimeInputHash(context.Background(), job, claimResponsePayload{
		HookRuntime: &contracts.HookRuntimeDecision{HookHash: "h1", HookShouldRun: true},
	})
	if err != nil {
		t.Fatalf("resolveRuntimeInputHash(A) error = %v", err)
	}
	hashB, okB, err := svc.resolveRuntimeInputHash(context.Background(), job, claimResponsePayload{
		HookRuntime: &contracts.HookRuntimeDecision{HookHash: "h2", HookShouldRun: true},
	})
	if err != nil {
		t.Fatalf("resolveRuntimeInputHash(B) error = %v", err)
	}
	if !okA || !okB {
		t.Fatalf("expected eligible hashes, got okA=%v okB=%v", okA, okB)
	}
	if hashA == hashB {
		t.Fatal("runtime input hash must differ for different hook runtime")
	}
}

func TestResolveRuntimeInputHash_DetectedStackAffectsHash(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	jobID := domaintypes.NewJobID()
	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      jobID,
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
		JobType: domaintypes.JobTypeMig,
	}}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	job := store.Job{ID: jobID, RunID: runID, RepoID: repoID, Attempt: 1, JobType: domaintypes.JobTypeMig}
	hashA, okA, err := svc.resolveRuntimeInputHash(context.Background(), job, claimResponsePayload{
		DetectedStack: &contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
	})
	if err != nil {
		t.Fatalf("resolveRuntimeInputHash(A) error = %v", err)
	}
	hashB, okB, err := svc.resolveRuntimeInputHash(context.Background(), job, claimResponsePayload{
		DetectedStack: &contracts.StackExpectation{Language: "java", Tool: "gradle", Release: "17"},
	})
	if err != nil {
		t.Fatalf("resolveRuntimeInputHash(B) error = %v", err)
	}
	if !okA || !okB {
		t.Fatalf("expected eligible hashes, got okA=%v okB=%v", okA, okB)
	}
	if hashA == hashB {
		t.Fatal("runtime input hash must differ for different detected stacks")
	}
}

func TestResolveUpstreamSBOMInputHash_UsesEffectiveSourceForMirroredPredecessor(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	currentID := domaintypes.NewJobID()
	predecessorID := domaintypes.NewJobID()
	sourceRunID := domaintypes.NewRunID()
	sourceJobID := domaintypes.NewJobID()
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	name := "mig-out"

	mirrorMeta, err := contracts.MarshalJobMeta(&contracts.JobMeta{
		Kind: "mig",
		CacheMirror: &contracts.CacheMirrorMetadata{
			SourceJobID: sourceJobID,
		},
	})
	if err != nil {
		t.Fatalf("marshal mirror meta: %v", err)
	}

	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:      predecessorID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			NextID:  &currentID,
			JobType: domaintypes.JobTypeSBOM,
			Status:  domaintypes.JobStatusSuccess,
		},
		{
			ID:      currentID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			JobType: domaintypes.JobTypePreGate,
		},
	}
	st.getJobResults = map[domaintypes.JobID]store.Job{
		predecessorID: {
			ID:      predecessorID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			Meta:    mirrorMeta,
		},
		sourceJobID: {
			ID:      sourceJobID,
			RunID:   sourceRunID,
			RepoID:  repoID,
			Attempt: 1,
			Meta:    []byte(`{"kind":"mig"}`),
		},
	}
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
		{Name: &name, Digest: &digest},
	}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	hash, required, available, err := svc.resolveUpstreamSBOMInputHash(context.Background(), store.Job{
		ID:      currentID,
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
	})
	if err != nil {
		t.Fatalf("resolveUpstreamSBOMInputHash() error = %v", err)
	}
	if !required || !available {
		t.Fatalf("required=%v available=%v, want true/true", required, available)
	}
	wantHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if hash != wantHash {
		t.Fatalf("hash = %q, want %q", hash, wantHash)
	}
	if !st.listArtifactBundlesByRunAndJob.called {
		t.Fatal("expected ListArtifactBundlesByRunAndJob to be called")
	}
	if got, want := st.listArtifactBundlesByRunAndJob.params.RunID, sourceRunID; got != want {
		t.Fatalf("artifact run_id = %s, want %s", got, want)
	}
	if st.listArtifactBundlesByRunAndJob.params.JobID == nil || *st.listArtifactBundlesByRunAndJob.params.JobID != sourceJobID {
		t.Fatalf("artifact job_id = %v, want %s", st.listArtifactBundlesByRunAndJob.params.JobID, sourceJobID)
	}
}

func TestReplayMirroredJobMeta_SetsSourceJobID(t *testing.T) {
	t.Parallel()

	sourceJobID := domaintypes.NewJobID()
	candidateRaw := []byte(`{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}`)

	raw, ok := replayMirroredJobMeta(sourceJobID, candidateRaw)
	if !ok {
		t.Fatal("expected replayMirroredJobMeta to succeed")
	}

	var meta contracts.JobMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal replay meta: %v", err)
	}
	if meta.CacheMirror == nil {
		t.Fatal("expected cache_mirror metadata")
	}
	if got, want := meta.CacheMirror.SourceJobID, sourceJobID; got != want {
		t.Fatalf("cache_mirror.source_job_id = %s, want %s", got, want)
	}
}

func TestReplayMirroredJobMeta_RejectsMirroredCandidate(t *testing.T) {
	t.Parallel()

	sourceJobID := domaintypes.NewJobID()
	candidateRaw := []byte(`{"kind":"mig","cache_mirror":{"source_job_id":"` + domaintypes.NewJobID().String() + `"}}`)

	if raw, ok := replayMirroredJobMeta(sourceJobID, candidateRaw); ok || raw != nil {
		t.Fatalf("expected mirrored candidate to be rejected, got ok=%v raw=%s", ok, string(raw))
	}
}

func TestReplayMirroredJobMeta_RejectsInvalidCandidateMeta(t *testing.T) {
	t.Parallel()

	sourceJobID := domaintypes.NewJobID()
	candidateRaw := []byte(`{"kind":"mig"`)

	if raw, ok := replayMirroredJobMeta(sourceJobID, candidateRaw); ok || raw != nil {
		t.Fatalf("expected invalid candidate meta to be rejected, got ok=%v raw=%s", ok, string(raw))
	}
}

func TestIsReplayStatusEligible(t *testing.T) {
	t.Parallel()

	one := int32(1)
	two := int32(2)

	cases := []struct {
		name     string
		jobType  domaintypes.JobType
		status   domaintypes.JobStatus
		exitCode *int32
		want     bool
	}{
		{name: "success always eligible", jobType: domaintypes.JobTypeMig, status: domaintypes.JobStatusSuccess, want: true},
		{name: "fail with exit one eligible for mig", jobType: domaintypes.JobTypeMig, status: domaintypes.JobStatusFail, exitCode: &one, want: true},
		{name: "fail with exit one ineligible for hook", jobType: domaintypes.JobTypeHook, status: domaintypes.JobStatusFail, exitCode: &one, want: false},
		{name: "fail with exit one ineligible for heal", jobType: domaintypes.JobTypeHeal, status: domaintypes.JobStatusFail, exitCode: &one, want: false},
		{name: "fail with exit above one ineligible", jobType: domaintypes.JobTypeMig, status: domaintypes.JobStatusFail, exitCode: &two, want: false},
		{name: "fail with missing exit ineligible", jobType: domaintypes.JobTypeMig, status: domaintypes.JobStatusFail, exitCode: nil, want: false},
		{name: "error ineligible", jobType: domaintypes.JobTypeMig, status: domaintypes.JobStatusError, exitCode: &one, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isReplayStatusEligible(tc.jobType, tc.status, tc.exitCode)
			if got != tc.want {
				t.Fatalf("isReplayStatusEligible(%q, %q, %v) = %v, want %v", tc.jobType, tc.status, tc.exitCode, got, tc.want)
			}
		})
	}
}

func TestHasReplayableSourceDiff(t *testing.T) {
	t.Parallel()

	baseJobID := domaintypes.NewJobID()
	baseRunID := domaintypes.NewRunID()
	baseRepoID := domaintypes.NewRepoID()

	tests := []struct {
		name    string
		job     store.Job
		setup   func(*jobStore)
		want    bool
		wantErr bool
	}{
		{
			name: "non-changing job is replayable without diff",
			job: store.Job{
				ID:      baseJobID,
				RunID:   baseRunID,
				RepoID:  baseRepoID,
				JobType: domaintypes.JobTypeSBOM,
			},
			want: true,
		},
		{
			name: "changing job with unchanged sha is not replayable",
			job: store.Job{
				ID:         baseJobID,
				RunID:      baseRunID,
				RepoID:     baseRepoID,
				JobType:    domaintypes.JobTypeMig,
				RepoShaIn:  "0123456789abcdef0123456789abcdef01234567",
				RepoShaOut: "0123456789abcdef0123456789abcdef01234567",
			},
			want: false,
		},
		{
			name: "changing job with changed sha and diff is replayable",
			job: store.Job{
				ID:         baseJobID,
				RunID:      baseRunID,
				RepoID:     baseRepoID,
				JobType:    domaintypes.JobTypeMig,
				RepoShaIn:  "0123456789abcdef0123456789abcdef01234567",
				RepoShaOut: "89abcdef0123456789abcdef0123456789abcdef",
			},
			setup: func(st *jobStore) {
				st.getLatestDiffByJob.val = store.Diff{}
			},
			want: true,
		},
		{
			name: "changing job with changed sha but missing diff is not replayable",
			job: store.Job{
				ID:         baseJobID,
				RunID:      baseRunID,
				RepoID:     baseRepoID,
				JobType:    domaintypes.JobTypeMig,
				RepoShaIn:  "0123456789abcdef0123456789abcdef01234567",
				RepoShaOut: "89abcdef0123456789abcdef0123456789abcdef",
			},
			setup: func(st *jobStore) {
				st.getLatestDiffByJob.err = pgx.ErrNoRows
			},
			want: false,
		},
		{
			name: "changing job with diff lookup error returns error",
			job: store.Job{
				ID:         baseJobID,
				RunID:      baseRunID,
				RepoID:     baseRepoID,
				JobType:    domaintypes.JobTypeMig,
				RepoShaIn:  "0123456789abcdef0123456789abcdef01234567",
				RepoShaOut: "89abcdef0123456789abcdef0123456789abcdef",
			},
			setup: func(st *jobStore) {
				st.getLatestDiffByJob.err = context.DeadlineExceeded
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			st := &jobStore{}
			if tc.setup != nil {
				tc.setup(st)
			}
			got, err := hasReplayableSourceDiff(context.Background(), st, tc.job)
			if (err != nil) != tc.wantErr {
				t.Fatalf("hasReplayableSourceDiff() err=%v wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got != tc.want {
				t.Fatalf("hasReplayableSourceDiff()=%v want %v", got, tc.want)
			}
		})
	}
}

func TestCopySBOMRowsFromSourceToTarget_ReplacesTargetRows(t *testing.T) {
	t.Parallel()

	sourceJobID := domaintypes.NewJobID()
	targetJobID := domaintypes.NewJobID()
	repoID := domaintypes.NewRepoID()

	st := &jobStore{
		listSBOMRowsByJobByID: map[domaintypes.JobID][]store.Sbom{
			targetJobID: {
				{JobID: targetJobID, RepoID: repoID, Lib: "old-lib", Ver: "0.1.0"},
			},
			sourceJobID: {
				{JobID: sourceJobID, RepoID: repoID, Lib: "a-lib", Ver: "1.0.0"},
				{JobID: sourceJobID, RepoID: repoID, Lib: "b-lib", Ver: "2.0.0"},
			},
		},
	}

	snapshot, err := copySBOMRowsFromSourceToTarget(context.Background(), st,
		store.Job{ID: sourceJobID, RepoID: repoID},
		store.Job{ID: targetJobID, RepoID: repoID},
	)
	if err != nil {
		t.Fatalf("copySBOMRowsFromSourceToTarget() error = %v", err)
	}
	if len(snapshot) != 1 || snapshot[0].Lib != "old-lib" {
		t.Fatalf("snapshot = %#v, want original target rows", snapshot)
	}
	if !st.deleteSBOMRowsByJob.called || len(st.deleteSBOMRowsByJob.calls) != 1 || st.deleteSBOMRowsByJob.calls[0] != targetJobID {
		t.Fatalf("DeleteSBOMRowsByJob calls = %#v, want [%s]", st.deleteSBOMRowsByJob.calls, targetJobID)
	}
	if got := len(st.upsertSBOMRow.calls); got != 2 {
		t.Fatalf("UpsertSBOMRow call count = %d, want 2", got)
	}
	for _, call := range st.upsertSBOMRow.calls {
		if call.JobID != targetJobID {
			t.Fatalf("upsert job_id = %s, want %s", call.JobID, targetJobID)
		}
		if call.RepoID != repoID {
			t.Fatalf("upsert repo_id = %s, want %s", call.RepoID, repoID)
		}
	}
}

func TestRestoreSBOMRowsForTarget_ReplacesWithSnapshot(t *testing.T) {
	t.Parallel()

	targetJobID := domaintypes.NewJobID()
	repoID := domaintypes.NewRepoID()
	rows := []store.Sbom{
		{JobID: targetJobID, RepoID: repoID, Lib: "x-lib", Ver: "1.2.3"},
		{JobID: targetJobID, RepoID: repoID, Lib: "y-lib", Ver: "4.5.6"},
	}
	st := &jobStore{}

	if err := restoreSBOMRowsForTarget(context.Background(), st, targetJobID, rows); err != nil {
		t.Fatalf("restoreSBOMRowsForTarget() error = %v", err)
	}
	if !st.deleteSBOMRowsByJob.called || len(st.deleteSBOMRowsByJob.calls) != 1 || st.deleteSBOMRowsByJob.calls[0] != targetJobID {
		t.Fatalf("DeleteSBOMRowsByJob calls = %#v, want [%s]", st.deleteSBOMRowsByJob.calls, targetJobID)
	}
	if got := len(st.upsertSBOMRow.calls); got != len(rows) {
		t.Fatalf("UpsertSBOMRow call count = %d, want %d", got, len(rows))
	}
	for i, call := range st.upsertSBOMRow.calls {
		if call.JobID != targetJobID {
			t.Fatalf("upsert[%d] job_id = %s, want %s", i, call.JobID, targetJobID)
		}
		if call.RepoID != rows[i].RepoID || call.Lib != rows[i].Lib || call.Ver != rows[i].Ver {
			t.Fatalf("upsert[%d] = %#v, want repo/lib/ver from %#v", i, call, rows[i])
		}
	}
}

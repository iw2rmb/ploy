package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestClaimService_Claim_ReturnsNoWorkWhenQueueEmpty(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	st := &jobStore{}
	st.claimJob.err = pgx.ErrNoRows
	st.getNode.val = store.Node{ID: nodeID}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	_, err := svc.Claim(context.Background(), nodeID)
	var noWork *ClaimNoWork
	if !errors.As(err, &noWork) {
		t.Fatalf("expected ClaimNoWork, got %T (%v)", err, err)
	}
}

func TestClaimService_Claim_SuccessBuildsPayloadAndTransitionsRepo(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &jobStore{
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
	}
	st.getNode.val = store.Node{ID: nodeID}
	st.getRun.val = store.Run{
		ID:        runID,
		SpecID:    specID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"img"}]}`)}
	st.claimJob.val = store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeID,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusRunning,
		JobType:     domaintypes.JobTypeMig,
		Meta:        []byte(`{}`),
	}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	result, err := svc.Claim(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if !st.updateRunRepoStatus.called {
		t.Fatal("expected UpdateRunRepoStatus to be called")
	}
	if st.unclaimJob.called {
		t.Fatal("expected UnclaimJob to not be called on successful claim")
	}
	if result.Payload.JobID != jobID {
		t.Fatalf("payload.job_id = %s, want %s", result.Payload.JobID, jobID)
	}
	if result.Payload.RunID != runID {
		t.Fatalf("payload.run_id = %s, want %s", result.Payload.RunID, runID)
	}
	if result.Payload.RepoURL == "" {
		t.Fatal("expected payload.repo_url to be populated")
	}
}

func TestClaimService_Claim_RequeuesClaimedJobWhenPayloadBuildFails(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &jobStore{
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
	}
	st.getNode.val = store.Node{ID: nodeID}
	st.getRun.val = store.Run{
		ID:        runID,
		SpecID:    specID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"img"}]}`)}
	st.claimJob.val = store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeID,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusRunning,
		JobType:     "invalid",
		Meta:        []byte(`{}`),
	}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	_, err := svc.Claim(context.Background(), nodeID)
	if err == nil {
		t.Fatal("expected Claim() to fail")
	}
	var internalErr *ClaimInternal
	if !errors.As(err, &internalErr) {
		t.Fatalf("expected ClaimInternal, got %T (%v)", err, err)
	}
	if !st.unclaimJob.called {
		t.Fatal("expected UnclaimJob to be called on payload build failure")
	}
	if got := st.unclaimJob.params.ID; got != jobID {
		t.Fatalf("unclaim job id = %s, want %s", got, jobID)
	}
	if got := st.unclaimJob.params.NodeID; got != nodeID {
		t.Fatalf("unclaim node id = %s, want %s", got, nodeID)
	}
	if st.updateRunRepoStatus.called {
		t.Fatal("expected UpdateRunRepoStatus to not be called when payload build fails")
	}
}

func TestClaimService_Claim_TerminalPayloadErrorCompletesClaimedJob(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()
	hookHash := "aa11bb22cc33"
	bundleID := "bundle_invalid_hook_for_claim"
	objKey := "spec_bundles/" + bundleID + "/bundle.tar.gz"

	st := &jobStore{
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
	}
	st.getNode.val = store.Node{ID: nodeID}
	st.getRun.val = store.Run{
		ID:        runID,
		SpecID:    specID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	st.getSpec.val = store.Spec{
		ID:   specID,
		Spec: []byte(`{"steps":[{"image":"img"}],"hooks":["` + hookHash + `"],"bundle_map":{"` + hookHash + `":"` + bundleID + `"}}`),
	}
	claimed := store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeID,
		Name:        "pre-gate-hook-000",
		Status:      domaintypes.JobStatusRunning,
		JobType:     domaintypes.JobTypeHook,
	}
	st.claimJob.val = claimed
	st.getJobResult = claimed
	st.getSpecBundle.val = store.SpecBundle{
		ID:        bundleID,
		ObjectKey: &objKey,
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: claimed.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}

	bs := bsmock.New()
	if _, err := bs.Put(context.Background(), objKey, "application/gzip", makeDirectContentBundleForClaimServiceTest(t, `id: invalid-hook
steps:
  - image: test:latest
    unknown_key: true
`)); err != nil {
		t.Fatalf("put invalid hook bundle: %v", err)
	}

	svc := NewClaimService(st, bs, &ConfigHolder{}, nil)
	_, err := svc.Claim(context.Background(), nodeID)
	var noWork *ClaimNoWork
	if !errors.As(err, &noWork) {
		t.Fatalf("expected ClaimNoWork after terminal payload error, got %T (%v)", err, err)
	}
	if st.unclaimJob.called {
		t.Fatal("expected UnclaimJob to not be called for terminal payload errors")
	}
	if !st.updateJobCompletion.called {
		t.Fatal("expected UpdateJobCompletion to be called for terminal payload errors")
	}
	if st.updateJobCompletion.params.Status != domaintypes.JobStatusError {
		t.Fatalf("update status = %s, want %s", st.updateJobCompletion.params.Status, domaintypes.JobStatusError)
	}
	if !st.updateRunRepoError.called {
		t.Fatal("expected UpdateRunRepoError to be called with terminal claim error details")
	}
}

func TestClaimService_Claim_CacheReplaySuccessTransitionsRepoToRunning(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &jobStore{
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
	}
	st.getNode.val = store.Node{ID: nodeID}
	st.getRun.val = store.Run{
		ID:        runID,
		SpecID:    specID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"img"}]}`)}
	st.claimJob.val = store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeID,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusRunning,
		JobType:     domaintypes.JobTypeMig,
		Meta:        []byte(`{}`),
	}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	svc.replayCachedOutcomeFn = func(context.Context, domaintypes.NodeID, store.Job, claimResponsePayload) (bool, error) {
		return true, nil
	}

	_, err := svc.Claim(context.Background(), nodeID)
	var noWork *ClaimNoWork
	if !errors.As(err, &noWork) {
		t.Fatalf("expected ClaimNoWork, got %T (%v)", err, err)
	}
	if !st.updateRunRepoStatus.called {
		t.Fatal("expected UpdateRunRepoStatus to be called")
	}
	if got := len(st.updateRunRepoStatus.calls); got != 1 {
		t.Fatalf("UpdateRunRepoStatus calls=%d, want 1", got)
	}
	if got := st.updateRunRepoStatus.calls[0].Status; got != domaintypes.RunRepoStatusRunning {
		t.Fatalf("UpdateRunRepoStatus[0].Status=%s, want %s", got, domaintypes.RunRepoStatusRunning)
	}
	if st.unclaimJob.called {
		t.Fatal("did not expect UnclaimJob on replay success")
	}
}

func TestClaimService_Claim_CacheReplayHardErrorUnclaimsJob(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &jobStore{
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
	}
	st.getNode.val = store.Node{ID: nodeID}
	st.getRun.val = store.Run{
		ID:        runID,
		SpecID:    specID,
		Status:    domaintypes.RunStatusStarted,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"img"}]}`)}
	st.claimJob.val = store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeID,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusRunning,
		JobType:     domaintypes.JobTypeMig,
		Meta:        []byte(`{}`),
	}

	svc := NewClaimService(st, nil, &ConfigHolder{}, nil)
	svc.replayCachedOutcomeFn = func(context.Context, domaintypes.NodeID, store.Job, claimResponsePayload) (bool, error) {
		return false, errors.New("replay crashed")
	}

	_, err := svc.Claim(context.Background(), nodeID)
	if err == nil {
		t.Fatal("expected Claim() to fail")
	}
	var internalErr *ClaimInternal
	if !errors.As(err, &internalErr) {
		t.Fatalf("expected ClaimInternal, got %T (%v)", err, err)
	}
	if !st.unclaimJob.called {
		t.Fatal("expected UnclaimJob to be called on non-recoverable replay error")
	}
	if !st.updateRunRepoStatus.called {
		t.Fatal("expected UpdateRunRepoStatus to be called for replay failure rollback")
	}
	if got := len(st.updateRunRepoStatus.calls); got != 2 {
		t.Fatalf("UpdateRunRepoStatus calls=%d, want 2", got)
	}
	if got := st.updateRunRepoStatus.calls[0].Status; got != domaintypes.RunRepoStatusRunning {
		t.Fatalf("UpdateRunRepoStatus[0].Status=%s, want %s", got, domaintypes.RunRepoStatusRunning)
	}
	if got := st.updateRunRepoStatus.calls[1].Status; got != domaintypes.RunRepoStatusQueued {
		t.Fatalf("UpdateRunRepoStatus[1].Status=%s, want %s", got, domaintypes.RunRepoStatusQueued)
	}
}

func makeDirectContentBundleForClaimServiceTest(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	body := []byte(content)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "content",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     int64(len(body)),
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

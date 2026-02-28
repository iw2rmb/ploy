package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestMaybeCreateHealingJobs_FirstAttemptCreatesJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	specID := domaintypes.NewSpecID()

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "migs-orw:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"by_error_kind": map[string]any{
					"infra": map[string]any{
						"retries": float64(2),
						"image":   "migs-codex:latest",
					},
				},
			},
			"router": map[string]any{
				"image": "migs-router:latest",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	st := &mockStore{
		getSpecResult: store.Spec{ID: specID, Spec: specBytes},
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "pre-gate",
				Status:      store.JobStatusFail,
				JobType:     domaintypes.JobTypePreGate.String(),
				Meta:        []byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","confidence":0.8,"reason":"docker socket missing"}}}`),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      store.JobStatusCreated,
				JobType:     domaintypes.JobTypeMod.String(),
				Meta:        []byte(`{}`),
			},
		},
	}
	successorID := st.listJobsByRunRepoAttemptResult[1].ID
	st.listJobsByRunRepoAttemptResult[0].NextID = &successorID

	run := store.Run{ID: runID, SpecID: specID, Status: store.RunStatusStarted}
	failed := st.listJobsByRunRepoAttemptResult[0]

	if err := maybeCreateHealingJobs(ctx, st, run, failed); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if st.createJobCallCount != 2 {
		t.Fatalf("expected 2 CreateJob calls (heal + re-gate), got %d", st.createJobCallCount)
	}
	if st.createJobParams[0].Name != "re-gate-1" {
		t.Fatalf("expected first created job to be re-gate-1 (tail-first FK-safe insert), got %q", st.createJobParams[0].Name)
	}
	if st.createJobParams[1].Name != "heal-1-0" {
		t.Fatalf("expected second created job to be heal-1-0, got %q", st.createJobParams[1].Name)
	}

	jobsByName := make(map[string]store.CreateJobParams)
	for _, p := range st.createJobParams {
		jobsByName[p.Name] = p
	}

	healJob, ok := jobsByName["heal-1-0"]
	if !ok {
		t.Fatalf("expected heal-1-0 job to be created")
	}
	if healJob.Status != store.JobStatusQueued {
		t.Fatalf("expected heal-1-0 status=Queued, got %s", healJob.Status)
	}
	if healJob.JobImage != "migs-codex:latest" {
		t.Fatalf("expected heal-1-0 image=migs-codex:latest, got %q", healJob.JobImage)
	}
	if healJob.NextID == nil {
		t.Fatalf("expected heal-1-0 next_id to be set")
	}

	reGateJob, ok := jobsByName["re-gate-1"]
	if !ok {
		t.Fatalf("expected re-gate-1 job to be created")
	}
	if reGateJob.Status != store.JobStatusCreated {
		t.Fatalf("expected re-gate-1 status=Created, got %s", reGateJob.Status)
	}
	if healJob.NextID == nil || *healJob.NextID != reGateJob.ID {
		t.Fatalf("expected heal to point to re-gate")
	}
	if reGateJob.NextID == nil || *reGateJob.NextID != successorID {
		t.Fatalf("expected re-gate to preserve original successor %s", successorID)
	}
	if len(st.updateJobNextIDParams) != 1 {
		t.Fatalf("expected one next_id rewiring update, got %d", len(st.updateJobNextIDParams))
	}
	if st.updateJobNextIDParams[0].ID != failed.ID {
		t.Fatalf("expected failed job %s to be rewired, got %s", failed.ID, st.updateJobNextIDParams[0].ID)
	}
	if st.updateJobNextIDParams[0].NextID == nil || *st.updateJobNextIDParams[0].NextID != healJob.ID {
		t.Fatalf("expected failed job to point to heal job %s", healJob.ID)
	}

	reGateMeta, err := contracts.UnmarshalJobMeta(reGateJob.Meta)
	if err != nil {
		t.Fatalf("unmarshal re-gate meta: %v", err)
	}
	if reGateMeta.Recovery == nil || reGateMeta.Recovery.ErrorKind != "infra" {
		t.Fatalf("expected re-gate recovery.error_kind=infra, got %#v", reGateMeta.Recovery)
	}
	if got, want := reGateMeta.Recovery.StrategyID, "infra-default"; got != want {
		t.Fatalf("re-gate recovery.strategy_id = %q, want %q", got, want)
	}

	healMeta, err := contracts.UnmarshalJobMeta(healJob.Meta)
	if err != nil {
		t.Fatalf("unmarshal heal meta: %v", err)
	}
	if healMeta.Recovery == nil || healMeta.Recovery.ErrorKind != "infra" {
		t.Fatalf("expected heal recovery.error_kind=infra, got %#v", healMeta.Recovery)
	}
}

func TestMaybeCreateHealingJobs_SecondAttemptUsesExistingHealJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	specID := domaintypes.NewSpecID()

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "migs-orw:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"by_error_kind": map[string]any{
					"infra": map[string]any{
						"retries": float64(3),
						"image":   "heal:latest",
					},
				},
			},
			"router": map[string]any{
				"image": "router:latest",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	st := &mockStore{
		getSpecResult: store.Spec{ID: specID, Spec: specBytes},
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "pre-gate",
				Status:      store.JobStatusFail,
				JobType:     domaintypes.JobTypePreGate.String(),
				Meta:        []byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "heal-1-0",
				Status:      store.JobStatusSuccess,
				JobType:     domaintypes.JobTypeHeal.String(),
				Meta:        []byte(`{}`),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "re-gate-1",
				Status:      store.JobStatusFail,
				JobType:     domaintypes.JobTypeReGate.String(),
				Meta:        []byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      store.JobStatusCreated,
				JobType:     domaintypes.JobTypeMod.String(),
				Meta:        []byte(`{}`),
			},
		},
	}
	heal1ID := st.listJobsByRunRepoAttemptResult[1].ID
	reGate1ID := st.listJobsByRunRepoAttemptResult[2].ID
	mod0ID := st.listJobsByRunRepoAttemptResult[3].ID
	st.listJobsByRunRepoAttemptResult[0].NextID = &heal1ID
	st.listJobsByRunRepoAttemptResult[1].NextID = &reGate1ID
	st.listJobsByRunRepoAttemptResult[2].NextID = &mod0ID

	run := store.Run{ID: runID, SpecID: specID, Status: store.RunStatusStarted}
	failed := st.listJobsByRunRepoAttemptResult[2]

	if err := maybeCreateHealingJobs(ctx, st, run, failed); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if st.createJobCallCount != 2 {
		t.Fatalf("expected 2 CreateJob calls (heal + re-gate), got %d", st.createJobCallCount)
	}
	if st.createJobParams[0].Name != "re-gate-2" {
		t.Fatalf("expected first healing job name re-gate-2, got %q", st.createJobParams[0].Name)
	}
	if st.createJobParams[0].JobType != domaintypes.JobTypeReGate.String() {
		t.Fatalf("expected first job JobType=re_gate, got %q", st.createJobParams[0].JobType)
	}
	if st.createJobParams[1].Name != "heal-2-0" {
		t.Fatalf("expected second healing job name heal-2-0, got %q", st.createJobParams[1].Name)
	}
	if st.createJobParams[1].JobType != domaintypes.JobTypeHeal.String() {
		t.Fatalf("expected second job JobType=heal, got %q", st.createJobParams[1].JobType)
	}
	if st.createJobParams[0].NextID == nil || *st.createJobParams[0].NextID != mod0ID {
		t.Fatalf("expected re-gate-2 to link back to original successor %s", mod0ID)
	}
}

func TestMaybeCreateHealingJobs_MixedClassificationCancelsRemaining(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	specID := domaintypes.NewSpecID()
	successorID := domaintypes.NewJobID()

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "migs-orw:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"by_error_kind": map[string]any{
					"infra": map[string]any{
						"retries": float64(2),
						"image":   "migs-codex:latest",
					},
				},
			},
			"router": map[string]any{
				"image": "migs-router:latest",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	failedID := domaintypes.NewJobID()
	st := &mockStore{
		getSpecResult: store.Spec{ID: specID, Spec: specBytes},
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          failedID,
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "pre-gate",
				Status:      store.JobStatusFail,
				JobType:     domaintypes.JobTypePreGate.String(),
				NextID:      &successorID,
				Meta:        []byte(`{"kind":"gate","gate":{"recovery":{"loop_kind":"healing","error_kind":"mixed","strategy_id":"mixed-default"}}}`),
			},
			{
				ID:          successorID,
				RunID:       runID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      store.JobStatusCreated,
				JobType:     domaintypes.JobTypeMod.String(),
				Meta:        []byte(`{"kind":"mig"}`),
			},
		},
	}

	run := store.Run{ID: runID, SpecID: specID, Status: store.RunStatusStarted}
	failed := st.listJobsByRunRepoAttemptResult[0]
	if err := maybeCreateHealingJobs(ctx, st, run, failed); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}
	if st.createJobCallCount != 0 {
		t.Fatalf("expected no healing jobs for mixed classification, got %d CreateJob calls", st.createJobCallCount)
	}
	if len(st.updateJobStatusCalls) != 1 {
		t.Fatalf("expected one cancelled successor, got %d calls", len(st.updateJobStatusCalls))
	}
	if st.updateJobStatusCalls[0].ID != successorID || st.updateJobStatusCalls[0].Status != store.JobStatusCancelled {
		t.Fatalf("unexpected cancellation params: %+v", st.updateJobStatusCalls[0])
	}
}

func TestMaybeCompleteMultiStepRun_FinishesWhenAllReposTerminal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()

	st := &mockStore{
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusSuccess, Count: 1},
			{Status: store.RunRepoStatusFail, Count: 1},
		},
	}

	run := store.Run{ID: runID, Status: store.RunStatusStarted}
	if err := maybeCompleteRunIfAllReposTerminal(ctx, st, nil, run, runID); err != nil {
		t.Fatalf("maybeCompleteRunIfAllReposTerminal returned error: %v", err)
	}

	if !st.updateRunStatusCalled {
		t.Fatalf("expected UpdateRunStatus to be called")
	}
	if st.updateRunStatusParams.ID != runID || st.updateRunStatusParams.Status != store.RunStatusFinished {
		t.Fatalf("unexpected UpdateRunStatus params: %+v", st.updateRunStatusParams)
	}
}

func TestLoadRecoveryArtifact_Success(t *testing.T) {
	t.Parallel()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	objKey := "artifacts/run/" + runID.String() + "/bundle/test.tar.gz"

	st := &mockStore{
		listArtifactBundlesMetaByRunAndJobResult: []store.ArtifactBundle{
			{
				RunID:     runID,
				JobID:     &jobID,
				ObjectKey: strPtr(objKey),
			},
		},
	}
	bs := bsmock.New()
	bundle := mustTarGzPayload(t, map[string][]byte{
		"out/prep-profile-candidate.json": []byte(`{"schema_version":1}`),
	})
	if _, err := bs.Put(context.Background(), objKey, "application/gzip", bundle); err != nil {
		t.Fatalf("put blob: %v", err)
	}

	bp := blobpersist.New(st, bs)
	raw, err := loadRecoveryArtifact(context.Background(), bp, runID, jobID, "/out/prep-profile-candidate.json")
	if err != nil {
		t.Fatalf("loadRecoveryArtifact error: %v", err)
	}
	if string(raw) != `{"schema_version":1}` {
		t.Fatalf("unexpected payload: %s", string(raw))
	}
}

func TestLoadRecoveryArtifact_TypedErrors(t *testing.T) {
	t.Parallel()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	st := &mockStore{}
	bp := blobpersist.New(st, bsmock.New())

	_, err := loadRecoveryArtifact(context.Background(), bp, runID, jobID, "/out/prep-profile-candidate.json")
	if !errors.Is(err, blobpersist.ErrRecoveryArtifactNotFound) {
		t.Fatalf("expected ErrRecoveryArtifactNotFound, got %v", err)
	}
}

func mustTarGzPayload(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
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

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/prep"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// maybeCreateHealingJobs inserts a heal -> re-gate chain after a failed gate job by rewiring next_id links.
func maybeCreateHealingJobs(
	ctx context.Context,
	st store.Store,
	bp *blobpersist.Service,
	run store.Run,
	failedJob store.Job,
) error {
	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   failedJob.RunID,
		RepoID:  failedJob.RepoID,
		Attempt: failedJob.Attempt,
	})
	if err != nil {
		return fmt.Errorf("list jobs for repo attempt: %w", err)
	}

	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, job := range jobs {
		jobsByID[job.ID] = job
	}

	// Refresh failed job from storage snapshot if present.
	if refreshed, ok := jobsByID[failedJob.ID]; ok {
		failedJob = refreshed
	}

	jobType := domaintypes.JobType(failedJob.JobType)
	if err := jobType.Validate(); err != nil {
		return fmt.Errorf("invalid job_type %q for failed job_id=%s: %w", failedJob.JobType, failedJob.ID, err)
	}
	if !isGateJobType(jobType) {
		slog.Debug("maybeCreateHealingJobs: not a gate job, skipping healing",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"job_type", jobType.String(),
		)
		return nil
	}

	recoveryMeta, detectedStack := resolveFailedGateRecoveryContext(failedJob)
	if recoveryMeta.ErrorKind == "mixed" || recoveryMeta.ErrorKind == "unknown" {
		slog.Info("maybeCreateHealingJobs: terminal recovery classification, canceling remaining linked jobs",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"error_kind", recoveryMeta.ErrorKind,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
	}

	specRow, err := st.GetSpec(ctx, run.SpecID)
	if err != nil {
		return fmt.Errorf("get spec: %w", err)
	}
	spec, err := contracts.ParseModsSpecJSON(specRow.Spec)
	if err != nil {
		return fmt.Errorf("parse run spec: %w", err)
	}

	healing := (*contracts.HealingSpec)(nil)
	if spec.BuildGate != nil {
		healing = spec.BuildGate.Healing
	}
	if healing == nil || len(healing.ByErrorKind) == 0 {
		slog.Debug("maybeCreateHealingJobs: no healing config, canceling remaining linked jobs",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
	}

	action, ok := healing.ByErrorKind[recoveryMeta.ErrorKind]
	if !ok {
		slog.Info("maybeCreateHealingJobs: no healing action for error_kind, canceling remaining linked jobs",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"error_kind", recoveryMeta.ErrorKind,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
	}
	if len(recoveryMeta.Expectations) == 0 && action.Expectations != nil {
		if b, err := json.Marshal(action.Expectations); err == nil {
			recoveryMeta.Expectations = b
		}
	}

	retries := action.Retries
	if retries <= 0 {
		retries = 1
	}

	baseGateID := resolveBaseGateID(failedJob, jobsByID)
	healingAttempts := countExistingHealingAttempts(baseGateID, jobsByID)
	healingAttemptNumber := healingAttempts + 1
	if healingAttemptNumber > retries {
		slog.Info("maybeCreateHealingJobs: healing retries exhausted",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"attempt", healingAttemptNumber,
			"max_retries", retries,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
	}

	healImage, err := action.Image.ResolveImage(detectedStack)
	if err != nil {
		return fmt.Errorf("resolve healing image for stack %q: %w", detectedStack, err)
	}

	reGateRecoveryMeta := cloneRecoveryMetadata(recoveryMeta)
	if shouldEvaluateInfraCandidate(recoveryMeta, action) {
		if reGateRecoveryMeta == nil {
			reGateRecoveryMeta = &contracts.BuildGateRecoveryMetadata{
				LoopKind:  recoveryMeta.LoopKind,
				ErrorKind: recoveryMeta.ErrorKind,
			}
		}
		artifactPath := contracts.PrepProfileCandidateArtifactPath
		if p, ok := resolveRecoveryCandidateArtifactPath(recoveryMeta.Expectations); ok {
			artifactPath = p
		}
		reGateRecoveryMeta.CandidateSchemaID = contracts.PrepProfileCandidateSchemaID
		reGateRecoveryMeta.CandidateArtifactPath = artifactPath
		evaluateAndAttachInfraCandidate(
			ctx,
			bp,
			run.ID,
			failedJob,
			jobsByID,
			reGateRecoveryMeta,
		)
	}

	reGateMetaBytes, err := contracts.MarshalJobMeta(&contracts.JobMeta{
		Kind:     contracts.JobKindGate,
		Recovery: reGateRecoveryMeta,
	})
	if err != nil {
		return fmt.Errorf("marshal re-gate job meta: %w", err)
	}
	healMetaBytes, err := contracts.MarshalJobMeta(&contracts.JobMeta{
		Kind:     contracts.JobKindMod,
		Recovery: cloneRecoveryMetadata(recoveryMeta),
	})
	if err != nil {
		return fmt.Errorf("marshal heal job meta: %w", err)
	}

	oldNext := failedJob.NextID
	healID := domaintypes.NewJobID()
	reGateID := domaintypes.NewJobID()

	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          reGateID,
		RunID:       failedJob.RunID,
		RepoID:      failedJob.RepoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     failedJob.Attempt,
		Name:        fmt.Sprintf("re-gate-%d", healingAttemptNumber),
		JobType:     domaintypes.JobTypeReGate.String(),
		JobImage:    "",
		Status:      store.JobStatusCreated,
		NextID:      oldNext,
		Meta:        reGateMetaBytes,
	})
	if err != nil {
		return fmt.Errorf("create re-gate job: %w", err)
	}

	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          healID,
		RunID:       failedJob.RunID,
		RepoID:      failedJob.RepoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     failedJob.Attempt,
		Name:        fmt.Sprintf("heal-%d-0", healingAttemptNumber),
		JobType:     domaintypes.JobTypeHeal.String(),
		JobImage:    healImage,
		Status:      store.JobStatusQueued,
		NextID:      &reGateID,
		Meta:        healMetaBytes,
	})
	if err != nil {
		return fmt.Errorf("create heal job: %w", err)
	}

	if err := st.UpdateJobNextID(ctx, store.UpdateJobNextIDParams{ID: failedJob.ID, NextID: &healID}); err != nil {
		return fmt.Errorf("rewire failed job next_id: %w", err)
	}

	slog.Info("maybeCreateHealingJobs: rewired chain",
		"run_id", failedJob.RunID,
		"failed_job_id", failedJob.ID,
		"heal_job_id", healID,
		"re_gate_job_id", reGateID,
		"old_next", oldNext,
		"attempt", healingAttemptNumber,
		"error_kind", recoveryMeta.ErrorKind,
		"strategy_id", recoveryMeta.StrategyID,
	)
	return nil
}

func resolveFailedGateRecoveryContext(failedJob store.Job) (*contracts.BuildGateRecoveryMetadata, contracts.ModStack) {
	meta := &contracts.BuildGateRecoveryMetadata{
		LoopKind:  "healing",
		ErrorKind: "unknown",
	}
	detectedStack := contracts.ModStackUnknown

	if len(failedJob.Meta) == 0 {
		return meta, detectedStack
	}

	jobMeta, err := contracts.UnmarshalJobMeta(failedJob.Meta)
	if err != nil {
		slog.Warn("maybeCreateHealingJobs: failed to parse failed gate job meta; defaulting recovery classification",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"error", err,
		)
		return meta, detectedStack
	}

	if jobMeta.Gate != nil {
		detectedStack = jobMeta.Gate.DetectedStack()
		if jobMeta.Gate.Recovery != nil {
			meta = cloneRecoveryMetadata(jobMeta.Gate.Recovery)
		}
	}
	if meta.ErrorKind == "unknown" && jobMeta.Recovery != nil {
		meta = cloneRecoveryMetadata(jobMeta.Recovery)
	}
	if meta.StrategyID == "" {
		meta.StrategyID = fmt.Sprintf("%s-default", meta.ErrorKind)
	}
	return meta, detectedStack
}

func cloneRecoveryMetadata(src *contracts.BuildGateRecoveryMetadata) *contracts.BuildGateRecoveryMetadata {
	if src == nil {
		return nil
	}
	out := *src
	if src.Confidence != nil {
		v := *src.Confidence
		out.Confidence = &v
	}
	if src.CandidatePromoted != nil {
		v := *src.CandidatePromoted
		out.CandidatePromoted = &v
	}
	if len(src.Expectations) > 0 {
		out.Expectations = append([]byte(nil), src.Expectations...)
	}
	if len(src.CandidatePrepProfile) > 0 {
		out.CandidatePrepProfile = append([]byte(nil), src.CandidatePrepProfile...)
	}
	return &out
}

func isGateJobType(jobType domaintypes.JobType) bool {
	return jobType == domaintypes.JobTypePreGate || jobType == domaintypes.JobTypePostGate || jobType == domaintypes.JobTypeReGate
}

func predecessorOf(jobID domaintypes.JobID, jobsByID map[domaintypes.JobID]store.Job) *store.Job {
	for _, candidate := range jobsByID {
		if candidate.NextID != nil && *candidate.NextID == jobID {
			c := candidate
			return &c
		}
	}
	return nil
}

func shouldEvaluateInfraCandidate(
	recoveryMeta *contracts.BuildGateRecoveryMetadata,
	action contracts.HealingActionSpec,
) bool {
	if recoveryMeta == nil || recoveryMeta.ErrorKind != "infra" {
		return false
	}
	if action.Expectations == nil {
		return false
	}
	for _, artifact := range action.Expectations.Artifacts {
		if strings.TrimSpace(artifact.Schema) == contracts.PrepProfileCandidateSchemaID {
			return true
		}
	}
	return false
}

func resolveRecoveryCandidateArtifactPath(expectations json.RawMessage) (string, bool) {
	if len(expectations) == 0 {
		return "", false
	}
	var ex struct {
		Artifacts []struct {
			Path   string `json:"path"`
			Schema string `json:"schema"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(expectations, &ex); err != nil {
		return "", false
	}
	for _, artifact := range ex.Artifacts {
		if strings.TrimSpace(artifact.Schema) != contracts.PrepProfileCandidateSchemaID {
			continue
		}
		path := strings.TrimSpace(artifact.Path)
		if path == "" {
			continue
		}
		return path, true
	}
	return "", false
}

func evaluateAndAttachInfraCandidate(
	ctx context.Context,
	bp *blobpersist.Service,
	runID domaintypes.RunID,
	failedJob store.Job,
	jobsByID map[domaintypes.JobID]store.Job,
	meta *contracts.BuildGateRecoveryMetadata,
) {
	if meta == nil {
		return
	}
	candidatePromoted := false
	meta.CandidatePromoted = &candidatePromoted
	path := strings.TrimSpace(meta.CandidateArtifactPath)
	if path == "" {
		path = contracts.PrepProfileCandidateArtifactPath
		meta.CandidateArtifactPath = path
	}
	prevHeal := resolvePreviousHealJob(failedJob, jobsByID)
	if prevHeal == nil {
		meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusMissing
		meta.CandidateValidationError = "candidate artifact unavailable: no previous heal job found"
		return
	}

	raw, err := loadRecoveryArtifact(ctx, bp, runID, prevHeal.ID, path)
	if err != nil {
		switch {
		case errors.Is(err, blobpersist.ErrRecoveryArtifactNotFound):
			meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusMissing
		case errors.Is(err, blobpersist.ErrRecoveryArtifactUnreadable):
			meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusUnavailable
		default:
			meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusInvalid
		}
		meta.CandidateValidationError = err.Error()
		return
	}
	if err := prep.ValidateProfileJSONForSchema(raw, contracts.PrepProfileCandidateSchemaID); err != nil {
		meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusInvalid
		meta.CandidateValidationError = err.Error()
		return
	}
	meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusValid
	meta.CandidateValidationError = ""
	meta.CandidatePrepProfile = append([]byte(nil), raw...)
}

func resolvePreviousHealJob(
	failedJob store.Job,
	jobsByID map[domaintypes.JobID]store.Job,
) *store.Job {
	prev := predecessorOf(failedJob.ID, jobsByID)
	if prev == nil {
		return nil
	}
	if domaintypes.JobType(prev.JobType) != domaintypes.JobTypeHeal {
		return nil
	}
	return prev
}

func resolveBaseGateID(failedJob store.Job, jobsByID map[domaintypes.JobID]store.Job) domaintypes.JobID {
	failedType := domaintypes.JobType(failedJob.JobType)
	if failedType != domaintypes.JobTypeReGate {
		return failedJob.ID
	}

	currentID := failedJob.ID
	for range len(jobsByID) {
		prev := predecessorOf(currentID, jobsByID)
		if prev == nil {
			break
		}
		prevType := domaintypes.JobType(prev.JobType)
		if prevType == domaintypes.JobTypePreGate || prevType == domaintypes.JobTypePostGate {
			return prev.ID
		}
		currentID = prev.ID
	}
	return failedJob.ID
}

func countExistingHealingAttempts(baseGateID domaintypes.JobID, jobsByID map[domaintypes.JobID]store.Job) int {
	base, ok := jobsByID[baseGateID]
	if !ok {
		return 0
	}

	attempts := 0
	seen := map[domaintypes.JobID]struct{}{}
	nextID := base.NextID
	for nextID != nil {
		if _, dup := seen[*nextID]; dup {
			break
		}
		seen[*nextID] = struct{}{}

		job, ok := jobsByID[*nextID]
		if !ok {
			break
		}
		jobType := domaintypes.JobType(job.JobType)
		if jobType == domaintypes.JobTypeHeal {
			attempts++
		}
		if jobType != domaintypes.JobTypeHeal && jobType != domaintypes.JobTypeReGate {
			break
		}
		nextID = job.NextID
	}
	return attempts
}

// cancelRemainingJobsAfterFailure cancels non-terminal jobs reachable from the failed job's successor chain.
func cancelRemainingJobsAfterFailure(
	ctx context.Context,
	st store.Store,
	failedJob store.Job,
) error {
	now := time.Now().UTC()

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   failedJob.RunID,
		RepoID:  failedJob.RepoID,
		Attempt: failedJob.Attempt,
	})
	if err != nil {
		return fmt.Errorf("list jobs for repo attempt: %w", err)
	}

	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, job := range jobs {
		jobsByID[job.ID] = job
	}

	nextID := failedJob.NextID
	if refreshed, ok := jobsByID[failedJob.ID]; ok {
		nextID = refreshed.NextID
	}

	seen := map[domaintypes.JobID]struct{}{}
	for nextID != nil {
		if _, dup := seen[*nextID]; dup {
			break
		}
		seen[*nextID] = struct{}{}

		job, ok := jobsByID[*nextID]
		if !ok {
			break
		}
		nextID = job.NextID

		switch job.Status {
		case store.JobStatusSuccess, store.JobStatusFail, store.JobStatusCancelled:
			continue
		}

		startedAt := job.StartedAt
		var durationMs int64
		if job.StartedAt.Valid {
			durationMs = now.Sub(job.StartedAt.Time).Milliseconds()
			if durationMs < 0 {
				durationMs = 0
			}
		}

		finishedAt := pgtype.Timestamptz{Time: now, Valid: true}
		if err := st.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:         job.ID,
			Status:     store.JobStatusCancelled,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			DurationMs: durationMs,
		}); err != nil {
			return fmt.Errorf("cancel job %s: %w", job.ID, err)
		}

		slog.Info("canceled linked job after failure",
			"run_id", failedJob.RunID,
			"failed_job_id", failedJob.ID,
			"job_id", job.ID,
		)
	}

	return nil
}

func loadRecoveryArtifact(
	ctx context.Context,
	bp *blobpersist.Service,
	runID domaintypes.RunID,
	healJobID domaintypes.JobID,
	artifactPath string,
) ([]byte, error) {
	if bp == nil {
		return nil, fmt.Errorf("load recovery artifact: blobpersist service is required")
	}
	return bp.LoadRecoveryArtifact(ctx, runID, healJobID, artifactPath)
}

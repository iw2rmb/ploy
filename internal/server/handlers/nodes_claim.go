package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// claimJobHandler allows nodes to claim a queued job for execution.
// Returns the claimed job with its parent run metadata or 204 No Content if no work is available.
//
// v1 status rules:
// - claimable jobs have status='Queued'; claimed jobs transition to 'Running'
// - normal jobs are claimable only when runs.status='Started'
// - MR jobs (job_type='mr') are claimable only when runs.status='Finished'
// - on first claim for a repo attempt, run_repos.status transitions Queued → Running
// - repo progression is attempt-scoped (run_id, repo_id, attempt)
//
// v1 response includes repo attribution:
// - repo_url: from mig_repos (since runs no longer have repo_url fields)
// - base_ref: from jobs.repo_base_ref (snapshot at job creation)
// - target_ref: from run_repos.repo_target_ref (snapshot at run_repos creation)
//
// Jobs are claimed from a single unified queue. There is no
// separate Build Gate queue or claim path — all job types (pre-gate, mig, heal,
// re-gate, post-gate) are consumed from the same queue.
// Jobs transition directly from 'Queued' to 'Running' on claim (no intermediate state).
func claimJobHandler(st store.Store, configHolder *ConfigHolder, eventsService *server.EventsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter using domain type helper.
		nodeID, err := parseParam[domaintypes.NodeID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Verify node exists before attempting to claim work.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if isNoRowsError(err) {
				httpErr(w, http.StatusNotFound, "node not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to check node: %s", safeErrorString(err))
			slog.Error("claim: node check failed", "node_id", nodeID, "err_type", fmt.Sprintf("%T", err), "err", safeErrorString(err))
			return
		}

		// Claim the next pending job. ClaimJob requires a non-empty nodeID.
		job, err := st.ClaimJob(r.Context(), nodeID)
		if err != nil {
			// No pending jobs available; return 204 No Content.
			if isNoRowsError(err) {
				w.WriteHeader(http.StatusNoContent)
				slog.Debug("claim: no work available", "node_id", nodeID)
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to claim job: %s", safeErrorString(err))
			slog.Error("claim: database error", "node_id", nodeID, "err_type", fmt.Sprintf("%T", err), "err", safeErrorString(err))
			return
		}

		run, err := st.GetRun(r.Context(), job.RunID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to get run for claimed job: %v", err)
			slog.Error("claim: get run failed for job", "node_id", nodeID, "job_id", job.ID, "err", err)
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: job.RunID, RepoID: job.RepoID})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to get run repo for claimed job: %v", err)
			slog.Error("claim: get run repo failed for job", "node_id", nodeID, "job_id", job.ID, "err", err)
			return
		}

		// v1 repo status transition: Queued → Running on first claim for repo attempt.
		// This is idempotent (already Running repos stay Running).
		// MR jobs must not affect run_repos.status.
		isMRJob := job.JobType == domaintypes.JobTypeMR.String()
		if !isMRJob && rr.Status == store.RunRepoStatusQueued {
			// The UpdateRunRepoStatus query sets started_at on first transition to Running.
			if err := st.UpdateRunRepoStatus(r.Context(), store.UpdateRunRepoStatusParams{
				RunID:  job.RunID,
				RepoID: job.RepoID,
				Status: store.RunRepoStatusRunning,
			}); err != nil {
				slog.Error("claim: failed to transition run repo to Running", "node_id", nodeID, "job_id", job.ID, "run_id", job.RunID, "repo_id", job.RepoID, "err", err)
			}
		}

		repoURL, err := repoURLForID(r.Context(), st, job.RepoID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to get repo for claimed job: %v", err)
			slog.Error("claim: get repo failed for job", "node_id", nodeID, "job_id", job.ID, "repo_id", job.RepoID, "err", err)
			return
		}

		spec, err := st.GetSpec(r.Context(), run.SpecID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to get spec for claimed job: %v", err)
			slog.Error("claim: get spec failed for job", "node_id", nodeID, "job_id", job.ID, "spec_id", run.SpecID, "err", err)
			return
		}

		// Build and send response with job and run information.
		if err := buildAndSendJobClaimResponse(w, r, st, configHolder, run, spec.Spec, rr, repoURL, job); err != nil {
			slog.Error("claim: failed to build response", "job_id", job.ID, "run_id", run.ID, "err", err)
			httpErr(w, http.StatusInternalServerError, "failed to build claim response: %v", err)
			return
		}
		slog.Info("job claimed",
			"job_id", job.ID, // Job IDs are KSUID strings.
			"job_name", job.Name,
			"run_id", run.ID, // Run IDs are KSUID strings.
			"next_id", job.NextID,
			"node_id", nodeID,
		)
	}
}

func isNoRowsError(err error) bool {
	if err == nil {
		return false
	}
	if err == pgx.ErrNoRows {
		return true
	}
	return errors.Is(err, pgx.ErrNoRows)
}

func safeErrorString(err error) (msg string) {
	if err == nil {
		return ""
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			msg = fmt.Sprintf("unprintable error (%T): panic while reading error string: %v", err, recovered)
		}
	}()
	return err.Error()
}

// buildAndSendJobClaimResponse constructs and sends the claim response for a job.
func buildAndSendJobClaimResponse(
	w http.ResponseWriter,
	r *http.Request,
	st store.Store,
	configHolder *ConfigHolder,
	run store.Run,
	spec []byte,
	runRepo store.RunRepo,
	repoURL string,
	job store.Job,
) error {
	jobType := domaintypes.JobType(job.JobType)
	if err := jobType.Validate(); err != nil {
		return fmt.Errorf("invalid claimed job job_type %q for job_id=%s: %w", job.JobType, job.ID, err)
	}

	mergedSpec, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:            spec,
		job:             job,
		jobType:         jobType,
		gitLab:          configHolder.GetGitLab(),
		globalEnv:       configHolder.GetGlobalEnv(),
		repoGateProfile: nil,
	})
	if err != nil {
		return err
	}
	recoveryCtx, err := buildRecoveryClaimContext(r.Context(), st, run.ID, job, jobType)
	if err != nil {
		return fmt.Errorf("build recovery context: %w", err)
	}

	// Response uses domain types for type-safe API output.
	// RunID uses JSON key "id" for wire compatibility with existing clients.
	resp := struct {
		RunID                  domaintypes.RunID               `json:"id"` // Run ID (KSUID); JSON key stays "id" for wire compatibility
		Name                   *string                         `json:"name,omitempty"`
		RepoID                 domaintypes.RepoID              `json:"repo_id"`
		Attempt                int32                           `json:"attempt"`
		JobID                  domaintypes.JobID               `json:"job_id"`    // Job ID (KSUID-backed)
		JobName                string                          `json:"job_name"`  // Job name (e.g., "pre-gate", "mig-0")
		JobType                domaintypes.JobType             `json:"job_type"`  // Job phase: pre_gate, mig, post_gate, heal, re_gate
		JobImage               string                          `json:"job_image"` // Container image for mig/heal jobs
		NextID                 *domaintypes.JobID              `json:"next_id"`
		RepoURL                string                          `json:"repo_url"`
		RepoGateProfileMissing bool                            `json:"repo_gate_profile_missing"`
		Status                 store.RunStatus                 `json:"status"`
		NodeID                 domaintypes.NodeID              `json:"node_id"` // Node ID (NanoID-backed)
		BaseRef                string                          `json:"base_ref"`
		TargetRef              string                          `json:"target_ref"`
		RepoSHAIn              string                          `json:"repo_sha_in,omitempty"`
		StartedAt              string                          `json:"started_at"`
		CreatedAt              string                          `json:"created_at"`
		Spec                   json.RawMessage                 `json:"spec,omitempty"`
		RecoveryContext        *contracts.RecoveryClaimContext `json:"recovery_context,omitempty"`
	}{
		RunID:                  run.ID,
		Name:                   nil,
		RepoID:                 job.RepoID,
		Attempt:                job.Attempt,
		JobID:                  job.ID,
		JobName:                job.Name,
		JobType:                jobType,
		JobImage:               job.JobImage,
		NextID:                 job.NextID,
		RepoURL:                repoURL,
		RepoGateProfileMissing: true,
		Status:                 run.Status,
		NodeID:                 nodeIDPtrOrZero(job.NodeID),
		BaseRef:                job.RepoBaseRef,
		TargetRef:              runRepo.RepoTargetRef,
		RepoSHAIn:              job.RepoShaIn,
		StartedAt:              run.StartedAt.Time.Format(time.RFC3339),
		CreatedAt:              run.CreatedAt.Time.Format(time.RFC3339),
		Spec:                   mergedSpec,
		RecoveryContext:        recoveryCtx,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("claim: encode response failed", "err", err)
	}
	return nil
}

func nodeIDPtrOrZero(id *domaintypes.NodeID) domaintypes.NodeID {
	if id == nil {
		return ""
	}
	return *id
}

func buildRecoveryClaimContext(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	job store.Job,
	jobType domaintypes.JobType,
) (*contracts.RecoveryClaimContext, error) {
	if jobType != domaintypes.JobTypeHeal && jobType != domaintypes.JobTypeReGate {
		return nil, nil
	}
	if len(job.Meta) == 0 {
		return nil, nil
	}
	jobMeta, err := contracts.UnmarshalJobMeta(job.Meta)
	if err != nil || jobMeta.Recovery == nil {
		return nil, nil
	}

	recovery := jobMeta.Recovery
	kind, ok := contracts.ParseRecoveryErrorKind(recovery.ErrorKind)
	if !ok {
		kind = contracts.DefaultRecoveryErrorKind()
	}
	selectedKind := kind.String()
	ctxPayload := &contracts.RecoveryClaimContext{
		LoopKind:          strings.TrimSpace(recovery.LoopKind),
		SelectedErrorKind: selectedKind,
		Expectations:      cloneRawJSON(recovery.Expectations),
	}
	if loopKind, ok := contracts.ParseRecoveryLoopKind(ctxPayload.LoopKind); ok {
		ctxPayload.LoopKind = loopKind.String()
	} else {
		ctxPayload.LoopKind = contracts.DefaultRecoveryLoopKind().String()
	}
	if jobType == domaintypes.JobTypeHeal && strings.TrimSpace(job.JobImage) != "" {
		ctxPayload.ResolvedHealingImage = strings.TrimSpace(job.JobImage)
	}

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   runID,
		RepoID:  job.RepoID,
		Attempt: job.Attempt,
	})
	if err != nil {
		return nil, fmt.Errorf("list jobs for recovery context: %w", err)
	}

	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, j := range jobs {
		jobsByID[j.ID] = j
	}

	gateJob, healJob := resolveRecoverySourceJobs(job, jobsByID)
	if healJob != nil && strings.TrimSpace(ctxPayload.ResolvedHealingImage) == "" {
		ctxPayload.ResolvedHealingImage = strings.TrimSpace(healJob.JobImage)
	}

	if gateJob != nil && len(gateJob.Meta) > 0 {
		if gateMeta, err := contracts.UnmarshalJobMeta(gateJob.Meta); err == nil && gateMeta.Gate != nil {
			if stack := gateMeta.Gate.DetectedStack(); stack != "" && stack != contracts.ModStackUnknown {
				ctxPayload.DetectedStack = stack
			}
			if logPayload := gateLogPayloadFromClaimMetadata(gateMeta.Gate); strings.TrimSpace(logPayload) != "" {
				ctxPayload.BuildGateLog = logPayload
			}
			if len(gateMeta.Gate.GeneratedGateProfile) > 0 {
				ctxPayload.GateProfile = cloneRawJSON(gateMeta.Gate.GeneratedGateProfile)
			}
		}
	}

	if kind, ok := contracts.ParseRecoveryErrorKind(ctxPayload.SelectedErrorKind); ok && contracts.IsInfraRecoveryErrorKind(kind) {
		schemaRaw, err := contracts.ReadGateProfileSchemaJSON()
		if err != nil {
			return nil, err
		}
		if !json.Valid(schemaRaw) {
			return nil, fmt.Errorf("gate profile schema JSON is invalid")
		}
		ctxPayload.GateProfileSchemaJSON = string(schemaRaw)
	}

	return ctxPayload, nil
}

func resolveRecoverySourceJobs(
	current store.Job,
	jobsByID map[domaintypes.JobID]store.Job,
) (*store.Job, *store.Job) {
	switch domaintypes.JobType(current.JobType) {
	case domaintypes.JobTypeHeal:
		heal := current
		prev := predecessorJob(current.ID, jobsByID)
		if prev != nil && isGateJobTypeForClaim(domaintypes.JobType(prev.JobType)) {
			return prev, &heal
		}
		return nil, &heal
	case domaintypes.JobTypeReGate:
		prev := predecessorJob(current.ID, jobsByID)
		if prev == nil {
			return nil, nil
		}
		if domaintypes.JobType(prev.JobType) == domaintypes.JobTypeHeal {
			heal := *prev
			prevGate := predecessorJob(prev.ID, jobsByID)
			if prevGate != nil && isGateJobTypeForClaim(domaintypes.JobType(prevGate.JobType)) {
				return prevGate, &heal
			}
			return nil, &heal
		}
		if isGateJobTypeForClaim(domaintypes.JobType(prev.JobType)) {
			return prev, nil
		}
	}
	return nil, nil
}

func predecessorJob(jobID domaintypes.JobID, jobsByID map[domaintypes.JobID]store.Job) *store.Job {
	for _, candidate := range jobsByID {
		if candidate.NextID != nil && *candidate.NextID == jobID {
			c := candidate
			return &c
		}
	}
	return nil
}

func isGateJobTypeForClaim(jobType domaintypes.JobType) bool {
	return jobType == domaintypes.JobTypePreGate ||
		jobType == domaintypes.JobTypePostGate ||
		jobType == domaintypes.JobTypeReGate
}

func gateLogPayloadFromClaimMetadata(gateMetadata *contracts.BuildGateStageMetadata) string {
	if gateMetadata == nil {
		return ""
	}
	if len(gateMetadata.LogFindings) == 0 {
		return ""
	}
	logPayload := strings.TrimSpace(gateMetadata.LogFindings[0].Message)
	if logPayload == "" {
		return ""
	}
	if !strings.HasSuffix(logPayload, "\n") {
		logPayload += "\n"
	}
	return logPayload
}

func cloneRawJSON(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), in...)
}

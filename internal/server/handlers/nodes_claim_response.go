package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type claimResponsePayload struct {
	RunID                  domaintypes.RunID                `json:"id"`
	Name                   *string                          `json:"name,omitempty"`
	RepoID                 domaintypes.RepoID               `json:"repo_id"`
	Attempt                int32                            `json:"attempt"`
	JobID                  domaintypes.JobID                `json:"job_id"`
	JobName                string                           `json:"job_name"`
	JobType                domaintypes.JobType              `json:"job_type"`
	JobImage               string                           `json:"job_image"`
	NextID                 *domaintypes.JobID               `json:"next_id"`
	RepoURL                string                           `json:"repo_url"`
	RepoGateProfileMissing bool                             `json:"repo_gate_profile_missing"`
	Status                 domaintypes.RunStatus            `json:"status"`
	NodeID                 domaintypes.NodeID               `json:"node_id"`
	BaseRef                string                           `json:"base_ref"`
	TargetRef              string                           `json:"target_ref"`
	RepoSHAIn              string                           `json:"repo_sha_in,omitempty"`
	StartedAt              string                           `json:"started_at"`
	CreatedAt              string                           `json:"created_at"`
	Spec                   json.RawMessage                  `json:"spec,omitempty"`
	RecoveryContext        *contracts.RecoveryClaimContext  `json:"recovery_context,omitempty"`
	GateSkip               *contracts.BuildGateSkipMetadata `json:"gate_skip,omitempty"`
	StepSkip               *contracts.MigStepSkipMetadata   `json:"step_skip,omitempty"`
}

func buildClaimResponsePayload(
	ctx context.Context,
	st store.Store,
	configHolder *ConfigHolder,
	run store.Run,
	spec []byte,
	runRepo store.RunRepo,
	repoURL string,
	job store.Job,
	gateProfileResolver GateProfileResolver,
) (claimResponsePayload, error) {
	jobType := domaintypes.JobType(job.JobType)
	if err := jobType.Validate(); err != nil {
		return claimResponsePayload{}, fmt.Errorf("invalid claimed job job_type %q for job_id=%s: %w", job.JobType, job.ID, err)
	}

	gitlabCfg := config.GitLabConfig{}
	globalEnv := map[string]GlobalEnvVar{}
	if configHolder != nil {
		gitlabCfg = configHolder.GetGitLab()
		globalEnv = configHolder.GetGlobalEnv()
	}

	var repoGateProfile []byte
	var gateSkip *contracts.BuildGateSkipMetadata
	if gateProfileResolver != nil && shouldResolveGateProfile(jobType) {
		phasePolicy, policyErr := gatePhasePolicyForJobSpec(spec, jobType)
		if policyErr != nil {
			return claimResponsePayload{}, policyErr
		}
		resolution, err := gateProfileResolver.ResolveGateProfileForJob(ctx, job, phasePolicy.LookupConstraints)
		if err != nil {
			return claimResponsePayload{}, fmt.Errorf("resolve gate profile: %w", err)
		}
		if resolution != nil {
			repoGateProfile = resolution.Payload
			if resolution.ExactHit && !phasePolicy.Always {
				gateSkip, err = resolveGateSkipMetadata(resolution.Payload, phasePolicy.Target, resolution.ProfileID)
				if err != nil {
					return claimResponsePayload{}, fmt.Errorf("resolve gate skip metadata: %w", err)
				}
			}
		}
	}

	mergedSpec, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:            spec,
		job:             job,
		jobType:         jobType,
		gitLab:          gitlabCfg,
		globalEnv:       globalEnv,
		repoGateProfile: repoGateProfile,
	})
	if err != nil {
		return claimResponsePayload{}, err
	}

	var stepSkip *contracts.MigStepSkipMetadata
	if jobType == domaintypes.JobTypeMod {
		stepSkip, err = resolveAndPersistModStepSkip(ctx, st, job, mergedSpec)
		if err != nil {
			return claimResponsePayload{}, fmt.Errorf("resolve step skip metadata: %w", err)
		}
	}

	recoveryCtx, err := buildRecoveryClaimContext(ctx, st, run.ID, job, jobType)
	if err != nil {
		return claimResponsePayload{}, fmt.Errorf("build recovery context: %w", err)
	}

	return claimResponsePayload{
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
		GateSkip:               gateSkip,
		StepSkip:               stepSkip,
	}, nil
}

func writeClaimResponse(w http.ResponseWriter, payload claimResponsePayload) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("claim: encode response failed", "err", err)
		return err
	}
	return nil
}

// buildAndSendJobClaimResponse constructs and sends the claim response for a job.
// Kept as a test helper wrapper around the response builder.
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
	gateProfileResolver GateProfileResolver,
) error {
	payload, err := buildClaimResponsePayload(r.Context(), st, configHolder, run, spec, runRepo, repoURL, job, gateProfileResolver)
	if err != nil {
		return err
	}
	return writeClaimResponse(w, payload)
}

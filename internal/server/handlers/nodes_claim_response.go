package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type claimResponsePayload struct {
	WorkType               string                               `json:"work_type"`
	RunID                  domaintypes.RunID                    `json:"id"`
	Name                   *string                              `json:"name,omitempty"`
	RepoID                 domaintypes.RepoID                   `json:"repo_id"`
	Attempt                int32                                `json:"attempt"`
	JobID                  domaintypes.JobID                    `json:"job_id"`
	JobName                string                               `json:"job_name"`
	JobType                domaintypes.JobType                  `json:"job_type"`
	ActionID               *domaintypes.JobID                   `json:"action_id,omitempty"`
	ActionType             string                               `json:"action_type,omitempty"`
	JobImage               string                               `json:"job_image"`
	NextID                 *domaintypes.JobID                   `json:"next_id"`
	RepoURL                string                               `json:"repo_url"`
	RepoGateProfileMissing bool                                 `json:"repo_gate_profile_missing"`
	Status                 domaintypes.RunStatus                `json:"status"`
	NodeID                 domaintypes.NodeID                   `json:"node_id"`
	BaseRef                string                               `json:"base_ref"`
	TargetRef              string                               `json:"target_ref"`
	RepoSHAIn              string                               `json:"repo_sha_in,omitempty"`
	StartedAt              string                               `json:"started_at"`
	CreatedAt              string                               `json:"created_at"`
	Spec                   json.RawMessage                      `json:"spec,omitempty"`
	SBOMContext            *contracts.SBOMJobMetadata           `json:"sbom_context,omitempty"`
	MigContext             *contracts.MigClaimContext           `json:"mig_context,omitempty"`
	HookContext            *contracts.HookClaimContext          `json:"hook_context,omitempty"`
	GateContext            *contracts.GateClaimContext          `json:"gate_context,omitempty"`
	JavaClasspathContext   *contracts.JavaClasspathClaimContext `json:"java_classpath_context,omitempty"`
	DetectedStack          *contracts.StackExpectation          `json:"detected_stack,omitempty"`
	RecoveryContext        *contracts.RecoveryClaimContext      `json:"recovery_context,omitempty"`
	HookRuntime            *contracts.HookRuntimeDecision       `json:"hook_runtime,omitempty"`
}

func buildClaimResponsePayload(
	ctx context.Context,
	st store.Store,
	bs blobstore.Store,
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
	globalEnv := map[string][]GlobalEnvVar{}
	var hydraOverlays map[string]*HydraJobConfig
	var bundleMap map[string]string
	if configHolder != nil {
		gitlabCfg = configHolder.GetGitLab()
		globalEnv = configHolder.GetGlobalEnvAll()
		hydraOverlays = configHolder.GetHydraOverlays()
		bundleMap = configHolder.GetBundleMap()
	}

	var repoGateProfile []byte
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
		}
	}

	mergedSpec, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:            spec,
		job:             job,
		jobType:         jobType,
		gitLab:          gitlabCfg,
		globalEnv:       globalEnv,
		repoGateProfile: repoGateProfile,
		hydraOverlays:   hydraOverlays,
		bundleMap:       bundleMap,
	})
	if err != nil {
		return claimResponsePayload{}, err
	}

	var sbomContext *contracts.SBOMJobMetadata
	var migContext *contracts.MigClaimContext
	var hookContext *contracts.HookClaimContext
	var gateContext *contracts.GateClaimContext

	if len(job.Meta) > 0 {
		if jobMeta, metaErr := contracts.UnmarshalJobMeta(job.Meta); metaErr == nil && jobMeta != nil {
			if jobType == domaintypes.JobTypeMig && jobMeta.MigStepIndex != nil {
				migContext = &contracts.MigClaimContext{StepIndex: *jobMeta.MigStepIndex}
			}
			if jobType == domaintypes.JobTypeHook {
				if jobMeta.HookIndex != nil {
					hookContext = &contracts.HookClaimContext{
						CycleName: jobMeta.HookCycleName,
						Source:    jobMeta.HookSource,
						Index:     *jobMeta.HookIndex,
					}
					hookContext.Normalize()
				}
			}
			if jobType == domaintypes.JobTypePreGate || jobType == domaintypes.JobTypePostGate || jobType == domaintypes.JobTypeReGate {
				if strings.TrimSpace(jobMeta.GateCycleName) != "" {
					gateContext = &contracts.GateClaimContext{CycleName: strings.TrimSpace(jobMeta.GateCycleName)}
				}
			}
		}
	}
	if jobType == domaintypes.JobTypeMig && migContext != nil {
		migSpec, parseErr := contracts.ParseMigSpecJSON(mergedSpec)
		if parseErr != nil {
			return claimResponsePayload{}, &ClaimJobTerminalError{
				Message: fmt.Sprintf("parse mig spec for in_from resolution (job %s)", job.ID),
				Err:     parseErr,
			}
		}
		resolvedInFrom, resolveErr := resolveMigInFromClaimEntries(ctx, st, job, migSpec, migContext.StepIndex)
		if resolveErr != nil {
			return claimResponsePayload{}, &ClaimJobTerminalError{
				Message: fmt.Sprintf("resolve in_from claim context for job %s", job.ID),
				Err:     resolveErr,
			}
		}
		migContext.InFrom = resolvedInFrom
	}

	if jobType == domaintypes.JobTypeSBOM {
		jobMeta, metaErr := contracts.UnmarshalJobMeta(job.Meta)
		if metaErr != nil {
			return claimResponsePayload{}, &ClaimJobTerminalError{
				Message: fmt.Sprintf("parse sbom job meta for job %s", job.ID),
				Err:     metaErr,
			}
		}
		if jobMeta.SBOM == nil {
			return claimResponsePayload{}, &ClaimJobTerminalError{
				Message: fmt.Sprintf("sbom job %s missing sbom meta context", job.ID),
				Err:     fmt.Errorf("meta.sbom is required"),
			}
		}
		ctx := *jobMeta.SBOM
		sbomContext = &ctx
	}

	recoveryCtx, err := buildRecoveryClaimContext(ctx, st, bs, run.ID, job, jobType)
	if err != nil {
		return claimResponsePayload{}, fmt.Errorf("build recovery context: %w", err)
	}
	detectedStack, err := resolveClaimDetectedStack(ctx, st, job)
	if err != nil {
		return claimResponsePayload{}, fmt.Errorf("resolve detected stack for claim: %w", err)
	}
	hookRuntime, err := resolveHookRuntimeDecision(ctx, st, bs, job, mergedSpec, jobType)
	if err != nil {
		return claimResponsePayload{}, fmt.Errorf("resolve hook runtime decision: %w", err)
	}
	javaClasspathContext, err := resolveJavaClasspathClaimContext(ctx, st, job)
	if err != nil {
		return claimResponsePayload{}, fmt.Errorf("resolve java classpath context: %w", err)
	}
	if jobType == domaintypes.JobTypeHook && hookContext != nil {
		upstreamSBOM, _, available, upstreamErr := resolveUpstreamSBOMBundleForJob(ctx, st, job)
		if upstreamErr != nil {
			return claimResponsePayload{}, fmt.Errorf("resolve hook upstream sbom bundle: %w", upstreamErr)
		}
		if available {
			hookContext.UpstreamSBOMArtifactID = strings.TrimSpace(upstreamSBOM.ArtifactID)
		}
	}

	return claimResponsePayload{
		WorkType:               "job",
		RunID:                  run.ID,
		Name:                   nil,
		RepoID:                 job.RepoID,
		Attempt:                job.Attempt,
		JobID:                  job.ID,
		JobName:                strings.TrimSpace(job.Name),
		JobType:                jobType,
		ActionID:               nil,
		ActionType:             "",
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
		SBOMContext:            sbomContext,
		MigContext:             migContext,
		HookContext:            hookContext,
		GateContext:            gateContext,
		JavaClasspathContext:   javaClasspathContext,
		DetectedStack:          detectedStack,
		RecoveryContext:        recoveryCtx,
		HookRuntime:            hookRuntime,
	}, nil
}

func buildActionClaimResponsePayload(
	run store.Run,
	spec []byte,
	runRepo store.RunRepo,
	repoURL string,
	action store.RunRepoAction,
) claimResponsePayload {
	return claimResponsePayload{
		WorkType:               "action",
		RunID:                  run.ID,
		Name:                   nil,
		RepoID:                 action.RepoID,
		Attempt:                action.Attempt,
		JobID:                  "",
		JobName:                "",
		JobType:                "",
		ActionID:               &action.ID,
		ActionType:             action.ActionType,
		JobImage:               "",
		NextID:                 nil,
		RepoURL:                repoURL,
		RepoGateProfileMissing: false,
		Status:                 run.Status,
		NodeID:                 nodeIDPtrOrZero(action.NodeID),
		BaseRef:                runRepo.RepoBaseRef,
		TargetRef:              runRepo.RepoTargetRef,
		RepoSHAIn:              "",
		StartedAt:              run.StartedAt.Time.Format(time.RFC3339),
		CreatedAt:              run.CreatedAt.Time.Format(time.RFC3339),
		Spec:                   spec,
		SBOMContext:            nil,
		MigContext:             nil,
		HookContext:            nil,
		GateContext:            nil,
		JavaClasspathContext:   nil,
		DetectedStack:          nil,
		RecoveryContext:        nil,
		HookRuntime:            nil,
	}
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
	bs blobstore.Store,
	configHolder *ConfigHolder,
	run store.Run,
	spec []byte,
	runRepo store.RunRepo,
	repoURL string,
	job store.Job,
	gateProfileResolver GateProfileResolver,
) error {
	payload, err := buildClaimResponsePayload(r.Context(), st, bs, configHolder, run, spec, runRepo, repoURL, job, gateProfileResolver)
	if err != nil {
		return err
	}
	return writeClaimResponse(w, payload)
}

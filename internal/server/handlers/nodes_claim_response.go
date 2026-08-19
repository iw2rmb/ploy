package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func buildJobClaimPayload(
	ctx context.Context,
	st store.Store,
	_ blobstore.Store,
	configHolder *ConfigHolder,
	run store.Run,
	spec []byte,
	repoURL string,
	job store.Job,
) (domainapi.NodeClaimResponse, error) {
	jobType := domaintypes.JobType(job.JobType)
	if err := jobType.Validate(); err != nil {
		return domainapi.NodeClaimResponse{}, fmt.Errorf("invalid claimed job job_type %q for job_id=%s: %w", job.JobType, job.ID, err)
	}

	globalEnv := map[string][]GlobalEnvVar{}
	var hydraOverlays map[string]*HydraJobConfig
	var bundleMap map[string]string
	if configHolder != nil {
		globalEnv = configHolder.GetGlobalEnvAll()
		hydraOverlays = configHolder.GetHydraOverlays()
		bundleMap = configHolder.GetBundleMap()
	}

	mergedSpec, err := mutateClaimSpec(claimSpecMutatorInput{
		spec:          spec,
		job:           job,
		jobType:       jobType,
		globalEnv:     globalEnv,
		hydraOverlays: hydraOverlays,
		bundleMap:     bundleMap,
	})
	if err != nil {
		return domainapi.NodeClaimResponse{}, err
	}

	var migContext *contracts.MigClaimContext
	var gateContext *contracts.GateClaimContext

	if len(job.Meta) > 0 {
		if jobMeta, metaErr := contracts.UnmarshalJobMeta(job.Meta); metaErr == nil && jobMeta != nil {
			if jobType == domaintypes.JobTypeMig && jobMeta.MigStepIndex != nil {
				migContext = &contracts.MigClaimContext{StepIndex: *jobMeta.MigStepIndex}
			}
			if jobType == domaintypes.JobTypePreGate || jobType == domaintypes.JobTypePostGate {
				if strings.TrimSpace(jobMeta.GateCycleName) != "" {
					gateContext = &contracts.GateClaimContext{CycleName: strings.TrimSpace(jobMeta.GateCycleName)}
				}
			}
		}
	}

	detectedStack, err := resolveClaimDetectedStack(ctx, st, job)
	if err != nil {
		return domainapi.NodeClaimResponse{}, fmt.Errorf("resolve detected stack for claim: %w", err)
	}
	commitSHA := strings.TrimSpace(run.SourceCommitSha)
	if commitSHA == "" {
		commitSHA = strings.TrimSpace(job.RepoShaIn)
	}

	return domainapi.NodeClaimResponse{
		RunID:         run.ID,
		Name:          nil,
		RepoID:        job.RepoID,
		Attempt:       job.Attempt,
		JobID:         job.ID,
		JobName:       strings.TrimSpace(job.Name),
		JobType:       jobType,
		JobImage:      job.JobImage,
		NextID:        job.NextID,
		RepoURL:       domaintypes.RepoURL(repoURL),
		Status:        run.Status,
		NodeID:        nodeIDPtrOrZero(job.NodeID),
		BaseRef:       domaintypes.GitRef(job.RepoBaseRef),
		CommitSHA:     domaintypes.CommitSHA(commitSHA),
		RepoSHAIn:     domaintypes.CommitSHA(job.RepoShaIn),
		StartedAt:     run.StartedAt.Time.Format(time.RFC3339),
		CreatedAt:     run.CreatedAt.Time.Format(time.RFC3339),
		Spec:          mergedSpec,
		MigContext:    migContext,
		GateContext:   gateContext,
		DetectedStack: detectedStack,
	}, nil
}

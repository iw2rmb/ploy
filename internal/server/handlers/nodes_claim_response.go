package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type workClaimPayload struct {
	RunID         domaintypes.RunID           `json:"id"`
	Name          *string                     `json:"name,omitempty"`
	RepoID        domaintypes.RepoID          `json:"repo_id"`
	Attempt       int32                       `json:"attempt"`
	JobID         domaintypes.JobID           `json:"job_id"`
	JobName       string                      `json:"job_name"`
	JobType       domaintypes.JobType         `json:"job_type"`
	JobImage      string                      `json:"job_image"`
	NextID        *domaintypes.JobID          `json:"next_id"`
	RepoURL       string                      `json:"repo_url"`
	Status        domaintypes.RunStatus       `json:"status"`
	NodeID        domaintypes.NodeID          `json:"node_id"`
	BaseRef       string                      `json:"base_ref"`
	CommitSHA     string                      `json:"commit_sha,omitempty"`
	RepoSHAIn     string                      `json:"repo_sha_in,omitempty"`
	StartedAt     string                      `json:"started_at"`
	CreatedAt     string                      `json:"created_at"`
	Spec          json.RawMessage             `json:"spec,omitempty"`
	MigContext    *contracts.MigClaimContext  `json:"mig_context,omitempty"`
	GateContext   *contracts.GateClaimContext `json:"gate_context,omitempty"`
	DetectedStack *contracts.StackExpectation `json:"detected_stack,omitempty"`
}

func buildJobClaimPayload(
	ctx context.Context,
	st store.Store,
	_ blobstore.Store,
	configHolder *ConfigHolder,
	run store.Run,
	spec []byte,
	repoURL string,
	job store.Job,
) (workClaimPayload, error) {
	jobType := domaintypes.JobType(job.JobType)
	if err := jobType.Validate(); err != nil {
		return workClaimPayload{}, fmt.Errorf("invalid claimed job job_type %q for job_id=%s: %w", job.JobType, job.ID, err)
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
		return workClaimPayload{}, err
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
		return workClaimPayload{}, fmt.Errorf("resolve detected stack for claim: %w", err)
	}
	commitSHA := strings.TrimSpace(run.SourceCommitSha)
	if commitSHA == "" {
		commitSHA = strings.TrimSpace(job.RepoShaIn)
	}

	return workClaimPayload{
		RunID:         run.ID,
		Name:          nil,
		RepoID:        job.RepoID,
		Attempt:       job.Attempt,
		JobID:         job.ID,
		JobName:       strings.TrimSpace(job.Name),
		JobType:       jobType,
		JobImage:      job.JobImage,
		NextID:        job.NextID,
		RepoURL:       repoURL,
		Status:        run.Status,
		NodeID:        nodeIDPtrOrZero(job.NodeID),
		BaseRef:       job.RepoBaseRef,
		CommitSHA:     commitSHA,
		RepoSHAIn:     job.RepoShaIn,
		StartedAt:     run.StartedAt.Time.Format(time.RFC3339),
		CreatedAt:     run.CreatedAt.Time.Format(time.RFC3339),
		Spec:          mergedSpec,
		MigContext:    migContext,
		GateContext:   gateContext,
		DetectedStack: detectedStack,
	}, nil
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
	repoURL string,
	job store.Job,
) error {
	payload, err := buildJobClaimPayload(r.Context(), st, bs, configHolder, run, spec, repoURL, job)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, payload)
	return nil
}

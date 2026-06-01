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

type claimResponsePayload struct {
	WorkType      string                      `json:"work_type"`
	RunID         domaintypes.RunID           `json:"id"`
	Name          *string                     `json:"name,omitempty"`
	RepoID        domaintypes.RepoID          `json:"repo_id"`
	Attempt       int32                       `json:"attempt"`
	JobID         domaintypes.JobID           `json:"job_id"`
	JobName       string                      `json:"job_name"`
	JobType       domaintypes.JobType         `json:"job_type"`
	ActionID      *domaintypes.JobID          `json:"action_id,omitempty"`
	ActionType    string                      `json:"action_type,omitempty"`
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

type nodeActionClaimResponsePayload struct {
	WorkType   string             `json:"work_type"`
	ActionID   domaintypes.JobID  `json:"action_id"`
	ActionType string             `json:"action_type"`
	NodeID     domaintypes.NodeID `json:"node_id"`
	StartedAt  string             `json:"started_at,omitempty"`
	CreatedAt  string             `json:"created_at"`
}

func buildClaimResponsePayload(
	ctx context.Context,
	st store.Store,
	_ blobstore.Store,
	configHolder *ConfigHolder,
	run store.Run,
	spec []byte,
	repoURL string,
	job store.Job,
) (claimResponsePayload, error) {
	jobType := domaintypes.JobType(job.JobType)
	if err := jobType.Validate(); err != nil {
		return claimResponsePayload{}, fmt.Errorf("invalid claimed job job_type %q for job_id=%s: %w", job.JobType, job.ID, err)
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
		return claimResponsePayload{}, err
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

	detectedStack, err := resolveClaimDetectedStack(ctx, st, job)
	if err != nil {
		return claimResponsePayload{}, fmt.Errorf("resolve detected stack for claim: %w", err)
	}
	commitSHA := strings.TrimSpace(run.SourceCommitSha)
	if commitSHA == "" {
		commitSHA = strings.TrimSpace(job.RepoShaIn)
	}

	return claimResponsePayload{
		WorkType:      "job",
		RunID:         run.ID,
		Name:          nil,
		RepoID:        job.RepoID,
		Attempt:       job.Attempt,
		JobID:         job.ID,
		JobName:       strings.TrimSpace(job.Name),
		JobType:       jobType,
		ActionID:      nil,
		ActionType:    "",
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

func buildActionClaimResponsePayload(
	spec []byte,
	run store.Run,
	repoURL string,
	action store.RunAction,
) claimResponsePayload {
	return claimResponsePayload{
		WorkType:      "action",
		RunID:         run.ID,
		Name:          nil,
		RepoID:        run.RepoID,
		Attempt:       action.Attempt,
		JobID:         "",
		JobName:       "",
		JobType:       "",
		ActionID:      &action.ID,
		ActionType:    action.ActionType,
		JobImage:      "",
		NextID:        nil,
		RepoURL:       repoURL,
		Status:        run.Status,
		NodeID:        nodeIDPtrOrZero(action.NodeID),
		BaseRef:       run.RepoBaseRef,
		RepoSHAIn:     "",
		StartedAt:     run.StartedAt.Time.Format(time.RFC3339),
		CreatedAt:     run.CreatedAt.Time.Format(time.RFC3339),
		Spec:          spec,
		MigContext:    nil,
		GateContext:   nil,
		DetectedStack: nil,
	}
}

func buildNodeActionClaimResponsePayload(action store.NodeAction) nodeActionClaimResponsePayload {
	payload := nodeActionClaimResponsePayload{
		WorkType:   "action",
		ActionID:   action.ID,
		ActionType: action.ActionType,
		NodeID:     action.NodeID,
	}
	if action.StartedAt.Valid {
		payload.StartedAt = action.StartedAt.Time.Format(time.RFC3339)
	}
	if action.CreatedAt.Valid {
		payload.CreatedAt = action.CreatedAt.Time.Format(time.RFC3339)
	}
	return payload
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
	payload, err := buildClaimResponsePayload(r.Context(), st, bs, configHolder, run, spec, repoURL, job)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, payload)
	return nil
}

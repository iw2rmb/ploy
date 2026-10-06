package api

import (
	"encoding/json"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// NodeClaimResponse is the control-plane contract returned to a node after a successful claim.
type NodeClaimResponse struct {
	ResumeCount   int                         `json:"resume_count,omitempty"`
	RunID         types.RunID                 `json:"id"`
	Name          *string                     `json:"name,omitempty"`
	RepoID        types.RepoID                `json:"repo_id"`
	Attempt       int32                       `json:"attempt"`
	JobID         types.JobID                 `json:"job_id"`
	JobName       string                      `json:"job_name"`
	JobType       types.JobType               `json:"job_type"`
	JobImage      string                      `json:"job_image"`
	NextID        *types.JobID                `json:"next_id"`
	RepoURL       types.RepoURL               `json:"repo_url"`
	Status        types.RunStatus             `json:"status"`
	NodeID        types.NodeID                `json:"node_id"`
	BaseRef       types.GitRef                `json:"base_ref"`
	CommitSHA     types.CommitSHA             `json:"commit_sha,omitempty"`
	RepoSHAIn     types.CommitSHA             `json:"repo_sha_in,omitempty"`
	StartedAt     string                      `json:"started_at"`
	CreatedAt     string                      `json:"created_at"`
	Spec          json.RawMessage             `json:"spec,omitempty"`
	MigContext    *contracts.MigClaimContext  `json:"mig_context,omitempty"`
	GateContext   *contracts.GateClaimContext `json:"gate_context,omitempty"`
	DetectedStack *contracts.StackExpectation `json:"detected_stack,omitempty"`
}

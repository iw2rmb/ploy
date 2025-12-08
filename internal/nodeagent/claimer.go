package nodeagent

import (
	"encoding/json"
	"net/http"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// ClaimManager periodically polls the server for work and executes claimed jobs.
// Nodes claim from a single unified jobs queue (FIFO by step_index); there is no
// separate Build Gate queue or claim path.
// Contains configuration, HTTP client, run controller, and backoff state for
// polling intervals when no work is available.
type ClaimManager struct {
	cfg        Config
	client     *http.Client
	controller RunController
	backoff    *backoff.StatefulBackoff
}

// ClaimResponse represents the response from POST /v1/nodes/{id}/claim.
// Returned by the server when a job is successfully claimed and assigned to this node.
// Contains the run metadata plus the claimed job's ID and name.
type ClaimResponse struct {
	ID        types.RunID     `json:"id"`         // Run ID
	JobID     types.JobID     `json:"job_id"`     // Claimed job ID
	JobName   string          `json:"job_name"`   // Job name (e.g., "pre-gate", "mod-0")
	ModType   string          `json:"mod_type"`   // Job phase: pre_gate, mod, post_gate, heal, re_gate
	ModImage  string          `json:"mod_image"`  // Container image for mod/heal jobs
	StepIndex types.StepIndex `json:"step_index"` // Job ordering index
	RepoURL   string          `json:"repo_url"`
	Status    string          `json:"status"`
	NodeID    types.NodeID    `json:"node_id"`
	BaseRef   string          `json:"base_ref"`
	TargetRef string          `json:"target_ref"`
	CommitSha *string         `json:"commit_sha,omitempty"`
	StartedAt string          `json:"started_at"`
	CreatedAt string          `json:"created_at"`
	Spec      json.RawMessage `json:"spec,omitempty"`
}

// NewClaimManager constructs a claim manager for the unified jobs queue.
// Nodes claim jobs from a single queue (FIFO by step_index); there is no
// separate Build Gate queue. Initializes backoff parameters for the claim
// loop polling interval.
func NewClaimManager(cfg Config, controller RunController) (*ClaimManager, error) {
	// Don't create HTTP client yet - defer until after bootstrap runs.
	// Client will be lazily initialized on first claim attempt.

	// Initialize shared backoff policy for claim loop polling.
	// Uses 250ms initial interval, 5s max cap matching previous behavior.
	backoffPolicy := backoff.ClaimLoopPolicy()

	return &ClaimManager{
		cfg:        cfg,
		client:     nil, // Will be initialized lazily
		controller: controller,
		backoff:    backoff.NewStatefulBackoff(backoffPolicy),
	}, nil
}

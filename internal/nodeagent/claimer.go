package nodeagent

import (
	"encoding/json"
	"net/http"

	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// ClaimManager periodically polls the server for work and executes claimed runs.
// Contains configuration, HTTP client, run controller, and buildgate executor.
// Manages backoff state for polling intervals when no work is available.
type ClaimManager struct {
	cfg           Config
	client        *http.Client
	controller    RunController
	buildgateExec *BuildGateExecutor
	backoff       *backoff.StatefulBackoff
}

// ClaimResponse represents the response from POST /v1/nodes/{id}/claim.
// Returned by the server when a run or step is successfully claimed and assigned to this node.
// StepIndex is present for multi-step runs where a specific step was claimed (multi-node execution).
type ClaimResponse struct {
	ID        string          `json:"id"`
	RepoURL   string          `json:"repo_url"`
	Status    string          `json:"status"`
	NodeID    string          `json:"node_id"`
	BaseRef   string          `json:"base_ref"`
	TargetRef string          `json:"target_ref"`
	CommitSha *string         `json:"commit_sha,omitempty"`
	StepIndex *int32          `json:"step_index,omitempty"` // Present for step-level claims
	StartedAt string          `json:"started_at"`
	CreatedAt string          `json:"created_at"`
	Spec      json.RawMessage `json:"spec,omitempty"`
}

// NewClaimManager constructs a claim manager with HTTP client and buildgate executor.
// Initializes backoff parameters for the claim loop polling interval.
func NewClaimManager(cfg Config, controller RunController) (*ClaimManager, error) {
	// Don't create HTTP client yet - defer until after bootstrap runs.
	// Client will be lazily initialized on first claim attempt.

	buildgateExec := NewBuildGateExecutor(cfg)

	// Initialize shared backoff policy for claim loop polling.
	// Uses 250ms initial interval, 5s max cap matching previous behavior.
	backoffPolicy := backoff.ClaimLoopPolicy()

	return &ClaimManager{
		cfg:           cfg,
		client:        nil, // Will be initialized lazily
		controller:    controller,
		buildgateExec: buildgateExec,
		backoff:       backoff.NewStatefulBackoff(backoffPolicy),
	}, nil
}

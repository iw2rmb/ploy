package nodeagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ClaimManager periodically polls the server for work and executes claimed runs.
// Contains configuration, HTTP client, run controller, and buildgate executor.
// Manages backoff state for polling intervals when no work is available.
type ClaimManager struct {
	cfg             Config
	client          *http.Client
	controller      RunController
	buildgateExec   *BuildGateExecutor
	backoffDuration time.Duration
	minBackoff      time.Duration
	maxBackoff      time.Duration
}

// ClaimResponse represents the response from POST /v1/nodes/{id}/claim.
// Returned by the server when a run is successfully claimed and assigned to this node.
type ClaimResponse struct {
	ID        string          `json:"id"`
	RepoURL   string          `json:"repo_url"`
	Status    string          `json:"status"`
	NodeID    string          `json:"node_id"`
	BaseRef   string          `json:"base_ref"`
	TargetRef string          `json:"target_ref"`
	CommitSha *string         `json:"commit_sha,omitempty"`
	StartedAt string          `json:"started_at"`
	CreatedAt string          `json:"created_at"`
	Spec      json.RawMessage `json:"spec,omitempty"`
}

// NewClaimManager constructs a claim manager with HTTP client and buildgate executor.
// Initializes backoff parameters for the claim loop polling interval.
func NewClaimManager(cfg Config, controller RunController) (*ClaimManager, error) {
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	buildgateExec := NewBuildGateExecutor(cfg)

	return &ClaimManager{
		cfg:             cfg,
		client:          client,
		controller:      controller,
		buildgateExec:   buildgateExec,
		backoffDuration: 0,
		minBackoff:      250 * time.Millisecond,
		maxBackoff:      5 * time.Second,
	}, nil
}

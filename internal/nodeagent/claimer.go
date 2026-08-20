package nodeagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// ClaimManager periodically polls the server for work and executes claimed jobs.
// Nodes claim from a single unified jobs queue (FIFO by next_id); there is no
// separate Build Gate queue or claim path.
// Contains configuration, HTTP client, run controller, and backoff state for
// polling intervals when no work is available.
type ClaimManager struct {
	cfg                Config
	client             *http.Client
	clientOnce         sync.Once // Ensures thread-safe lazy HTTP client initialization
	clientErr          error     // Stores initialization error from clientOnce
	statusUploader     *baseUploader
	statusUploaderOnce sync.Once
	statusUploaderErr  error
	eventUploader      *baseUploader
	eventUploaderOnce  sync.Once
	eventUploaderErr   error
	controller         RunController
	startupReconciler  *startupCrashReconciler
	startupOnce        sync.Once
	startupErr         error
	backoff            *backoff.StatefulBackoff
}

// NewClaimManager constructs a claim manager for the unified jobs queue.
// Nodes claim jobs from a single queue (FIFO by next_id); there is no
// separate Build Gate queue. Initializes backoff parameters for the claim
// loop polling interval.
func NewClaimManager(cfg Config, controller RunController) (*ClaimManager, error) {
	// Don't create HTTP client yet - defer until after bootstrap runs.
	// Client will be lazily initialized on first claim attempt.
	startupReconciler, err := newStartupCrashReconciler()
	if err != nil {
		return nil, fmt.Errorf("create startup crash reconciler: %w", err)
	}

	// Initialize shared backoff policy for claim loop polling.
	// Uses 250ms initial interval, 5s max cap matching previous behavior.
	backoffPolicy := backoff.ClaimLoopPolicy()

	return &ClaimManager{
		cfg:               cfg,
		client:            nil, // Will be initialized lazily
		controller:        controller,
		startupReconciler: startupReconciler,
		backoff:           backoff.NewStatefulBackoff(backoffPolicy),
	}, nil
}

// parseSpec parses and validates the canonical migration specification.
func parseSpec(spec json.RawMessage) (*contracts.MigSpec, error) {
	if len(spec) == 0 {
		return nil, nil
	}
	return contracts.ParseMigSpecJSON(spec)
}

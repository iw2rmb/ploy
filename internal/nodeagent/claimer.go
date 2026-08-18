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

// parseSpec parses a spec JSON payload into environment variables and typed options.
// It uses the canonical contracts.ParseMigSpecJSON parser for structured
// validation, then converts to the internal RunOptions format.
//
// ## Return Values
//
// Returns:
//   - env: map[string]string containing merged environment variables (global env,
//     plus step env for single-step runs; multi-step step env is handled per-step
//     during manifest building).
//   - typedOpts: RunOptions with typed accessors for all understood option keys.
//   - err: non-nil if spec parsing fails.
//
// If the spec is empty, returns an empty env map and zero RunOptions with nil error.
func parseSpec(spec json.RawMessage) (map[string]string, RunOptions, error) {
	env := map[string]string{}
	var typedOpts RunOptions
	if len(spec) == 0 {
		return env, typedOpts, nil
	}

	// Parse using the canonical parser for structural validation.
	migsSpec, err := contracts.ParseMigSpecJSON(spec)
	if err != nil {
		return env, typedOpts, err
	}

	// Derive env with legacy semantics:
	// - Global env applies to every step.
	// - For single-step runs, step env is merged into env (step overrides).
	// - For multi-step runs, env contains only the global env; step env is applied
	//   at manifest build time via typedOpts.Steps[stepIndex].Env.
	env = envFromSpec(migsSpec)

	// Direct conversion from typed MigSpec to RunOptions.
	typedOpts = runOptionsFromSpec(migsSpec)

	return env, typedOpts, nil
}

func envFromSpec(spec *contracts.MigSpec) map[string]string {
	if spec == nil {
		return map[string]string{}
	}

	env := make(map[string]string, len(spec.Envs))
	for k, v := range spec.Envs {
		env[k] = v
	}

	if len(spec.Steps) == 1 {
		for k, v := range spec.Steps[0].Envs {
			env[k] = v
		}
	}

	return env
}

package nodeagent

import (
    "context"
    "errors"
    "fmt"
    "log/slog"
    "sync"
    "time"
)

// Agent coordinates the node agent's HTTP server and heartbeat manager.
type Agent struct {
	cfg        Config
	server     *Server
	heartbeat  *HeartbeatManager
	controller *runController
}

// New constructs a new node agent.
func New(cfg Config) (*Agent, error) {
	controller := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	server, err := NewServer(cfg, controller)
	if err != nil {
		return nil, fmt.Errorf("create server: %w", err)
	}

	heartbeat, err := NewHeartbeatManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("create heartbeat manager: %w", err)
	}

	return &Agent{
		cfg:        cfg,
		server:     server,
		heartbeat:  heartbeat,
		controller: controller,
	}, nil
}

// Run starts the node agent and blocks until the context is canceled.
func (a *Agent) Run(ctx context.Context) error {
    if err := a.server.Start(ctx); err != nil {
        return fmt.Errorf("start server: %w", err)
    }
    slog.Info("node http server listening", "addr", a.server.Address())

	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.heartbeat.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			select {
			case errCh <- fmt.Errorf("heartbeat: %w", err):
			default:
			}
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.server.Stop(shutdownCtx); err != nil {
		return fmt.Errorf("stop server: %w", err)
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// runController implements the RunController interface for managing runs.
type runController struct {
	mu   sync.Mutex
	cfg  Config
	runs map[string]*runContext
}

type runContext struct {
	runID  string
	cancel context.CancelFunc
}

// StartRun accepts a run start request and initiates execution.
func (r *runController) StartRun(ctx context.Context, req StartRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.runs[req.RunID]; exists {
		return fmt.Errorf("run %s already exists", req.RunID)
	}

	// Create a cancellable context for this run.
	runCtx, cancel := context.WithCancel(context.Background())
	r.runs[req.RunID] = &runContext{
		runID:  req.RunID,
		cancel: cancel,
	}

	// In the skeleton, we just accept the run without executing it.
	// Actual execution will be implemented in subsequent tasks.
	go r.executeRun(runCtx, req)

	return nil
}

// StopRun cancels a running job.
func (r *runController) StopRun(ctx context.Context, req StopRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, exists := r.runs[req.RunID]
	if !exists {
		return fmt.Errorf("run %s not found", req.RunID)
	}

	run.cancel()
	delete(r.runs, req.RunID)

	return nil
}

func (r *runController) executeRun(ctx context.Context, req StartRunRequest) {
	// Placeholder for actual run execution logic.
	// This will be implemented in the "Node execution contract" task.
	<-ctx.Done()
}

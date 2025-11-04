package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Agent coordinates the node agent's HTTP server, heartbeat manager, and claim loop.
type Agent struct {
	cfg        Config
	server     *Server
	heartbeat  *HeartbeatManager
	claimer    *ClaimManager
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

	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		return nil, fmt.Errorf("create claim manager: %w", err)
	}

	return &Agent{
		cfg:        cfg,
		server:     server,
		heartbeat:  heartbeat,
		claimer:    claimer,
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
	errCh := make(chan error, 2)

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

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.claimer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			select {
			case errCh <- fmt.Errorf("claim loop: %w", err):
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

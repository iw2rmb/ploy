package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/events"
	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/api/metrics"
	"github.com/iw2rmb/ploy/internal/api/pki"
	"github.com/iw2rmb/ploy/internal/api/scheduler"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/store/ttlworker"
)

// run executes the main server loop and blocks until the context is canceled.
func run(ctx context.Context, cfg config.Config, configPath string, st store.Store, authorizer *auth.Authorizer) error {
	// Initialize PKI manager for certificate renewal.
	rotator := pki.NewDefaultRotator(slog.Default())
	pkiManager, err := pki.New(pki.Options{
		Config:  cfg.PKI,
		Rotator: rotator,
	})
	if err != nil {
		return fmt.Errorf("create pki manager: %w", err)
	}

	// Initialize config watcher for hot-reload.
	configWatcher, err := config.NewWatcher(config.WatcherOptions{
		Path:   configPath,
		Logger: slog.Default(),
	})
	if err != nil {
		return fmt.Errorf("create config watcher: %w", err)
	}

	// Subscribe PKI manager to config changes.
	configWatcher.Subscribe(pkiManager)

	// Initialize TTL worker.
	ttlWorker, err := ttlworker.New(ttlworker.Options{
		Store:          st,
		TTL:            cfg.Scheduler.TTL,
		Interval:       cfg.Scheduler.TTLInterval,
		Logger:         slog.Default(),
		DropPartitions: cfg.Scheduler.DropPartitions,
	})
	if err != nil {
		return fmt.Errorf("create ttl worker: %w", err)
	}

	// Initialize events service for SSE fanout.
	eventsService, err := events.New(events.Options{
		BufferSize:  32,
		HistorySize: 256,
		Logger:      slog.Default(),
		Store:       st,
	})
	if err != nil {
		return fmt.Errorf("create events service: %w", err)
	}

	// Initialize scheduler and register background tasks.
	sched := scheduler.New()
	if ttlWorker != nil {
		sched.AddTask(ttlWorker)
	}

	// Start PKI manager.
	if err := pkiManager.Start(ctx); err != nil {
		return fmt.Errorf("start pki manager: %w", err)
	}

	// Start config watcher.
	if err := configWatcher.Start(ctx); err != nil {
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start config watcher: %w", err)
	}

	// Start events service.
	if err := eventsService.Start(ctx); err != nil {
		_ = configWatcher.Stop(context.Background())
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start events service: %w", err)
	}

	// Start scheduler.
	if err := sched.Start(ctx); err != nil {
		_ = eventsService.Stop(context.Background())
		_ = configWatcher.Stop(context.Background())
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start scheduler: %w", err)
	}

	// Initialize HTTP server for API endpoints.
	httpSrv, err := httpserver.New(httpserver.Options{
		Config:     cfg.HTTP,
		Authorizer: authorizer,
	})
	if err != nil {
		return fmt.Errorf("create http server: %w", err)
	}

	// Register health endpoint.
	httpSrv.HandleFunc("/health", healthHandler)

	// Register PKI sign endpoint (admin-only).
	httpSrv.HandleFunc("POST /v1/pki/sign", pkiSignHandler(st), auth.RoleCLIAdmin)

	// Register repos endpoints (control plane).
	httpSrv.HandleFunc("POST /v1/repos", createRepoHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("GET /v1/repos", listReposHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("GET /v1/repos/{id}", getRepoHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("DELETE /v1/repos/{id}", deleteRepoHandler(st), auth.RoleControlPlane)

	// Register mods endpoints (control plane).
	httpSrv.HandleFunc("POST /v1/mods/crud", createModHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("GET /v1/mods/crud", listModsHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("GET /v1/mods/crud/{id}", getModHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("DELETE /v1/mods/crud/{id}", deleteModHandler(st), auth.RoleControlPlane)

	// Register runs endpoints (control plane).
	httpSrv.HandleFunc("POST /v1/runs", createRunHandler(st), auth.RoleControlPlane)
	// Support both query (?id=) and RESTful path (/v1/runs/{id}) for basic run view.
	httpSrv.HandleFunc("GET /v1/runs", getRunHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("GET /v1/runs/{id}", getRunHandler(st), auth.RoleControlPlane)
	// Explicit timing subresource for clarity and OpenAPI alignment.
	httpSrv.HandleFunc("GET /v1/runs/{id}/timing", getRunTimingHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("DELETE /v1/runs/{id}", deleteRunHandler(st), auth.RoleControlPlane)
	// SSE events endpoint for run log streaming.
	httpSrv.HandleFunc("GET /v1/runs/{id}/events", getRunEventsHandler(st, eventsService), auth.RoleControlPlane)

	// Legacy /v1/jobs endpoints (aliases for /v1/runs endpoints).
	// These are maintained for backwards compatibility with existing CLI/tests.
	httpSrv.HandleFunc("GET /v1/jobs", getRunHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("GET /v1/jobs/{id}", getRunHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("GET /v1/jobs/{id}/logs/stream", getRunEventsHandler(st, eventsService), auth.RoleControlPlane)
	httpSrv.HandleFunc("POST /v1/jobs/{id}/retry", retryRunHandler(st), auth.RoleControlPlane)
	// Note: /v1/jobs/{id}/heartbeat and /v1/jobs/{id}/complete are deprecated.
	// Modern nodes use /v1/nodes/{id}/heartbeat and /v1/nodes/{id}/complete instead.

	// Legacy /v1/mods/{ticket}/logs/stream endpoint (SSE stream for Mods ticket logs).
	httpSrv.HandleFunc("GET /v1/mods/{ticket}/logs/stream", getRunEventsHandler(st, eventsService), auth.RoleControlPlane)

	// Register node heartbeat endpoint (node agents).
	httpSrv.HandleFunc("POST /v1/nodes/{id}/heartbeat", heartbeatHandler(st), auth.RoleWorker)
	// Register node claim endpoint (node agents pull work).
	httpSrv.HandleFunc("POST /v1/nodes/{id}/claim", claimRunHandler(st), auth.RoleWorker)
	// Register node acknowledgement endpoint (node agents acknowledge run start).
	httpSrv.HandleFunc("POST /v1/nodes/{id}/ack", ackRunStartHandler(st), auth.RoleWorker)
	// Register node completion endpoint (node agents mark run as finished).
	httpSrv.HandleFunc("POST /v1/nodes/{id}/complete", completeRunHandler(st), auth.RoleWorker)
	// Register node events endpoint (node agents).
	httpSrv.HandleFunc("POST /v1/nodes/{id}/events", createNodeEventsHandler(st, eventsService), auth.RoleWorker)
	// Register node logs endpoint (node agents stream gzipped log chunks).
	httpSrv.HandleFunc("POST /v1/nodes/{id}/logs", createNodeLogsHandler(st), auth.RoleWorker)
	// Register node diff upload endpoint (node agents).
	httpSrv.HandleFunc("POST /v1/nodes/{id}/stage/{stage}/diff", createDiffHandler(st), auth.RoleWorker)
	// Register node artifact bundle upload endpoint (node agents).
	httpSrv.HandleFunc("POST /v1/nodes/{id}/stage/{stage}/artifact", createArtifactBundleHandler(st), auth.RoleWorker)

	// Initialize metrics server.
	metricsSrv := metrics.New(metrics.Options{
		Listen: cfg.Metrics.Listen,
	})

	// Start HTTP server.
	if err := httpSrv.Start(ctx); err != nil {
		// Ensure background tasks are stopped on failure.
		_ = sched.Stop(context.Background())
		_ = eventsService.Stop(context.Background())
		_ = configWatcher.Stop(context.Background())
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start http server: %w", err)
	}

	// Start metrics server.
	if err := metricsSrv.Start(ctx); err != nil {
		// Stop HTTP server on failure to start metrics.
		_ = httpSrv.Stop(context.Background())
		// Stop scheduler to avoid leaking background goroutines.
		_ = sched.Stop(context.Background())
		_ = eventsService.Stop(context.Background())
		_ = configWatcher.Stop(context.Background())
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start metrics server: %w", err)
	}

	slog.Info("ployd servers started",
		"api", httpSrv.Addr(),
		"metrics", metricsSrv.Addr(),
	)

	// Wait for shutdown signal.
	<-ctx.Done()

	// Create a timeout context for graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Info("graceful shutdown initiated", "timeout", "10s")

	// Stop scheduler.
	if err := sched.Stop(shutdownCtx); err != nil {
		slog.Error("stop scheduler", "err", err)
	}

	// Stop events service.
	if err := eventsService.Stop(shutdownCtx); err != nil {
		slog.Error("stop events service", "err", err)
	}

	// Stop config watcher.
	if err := configWatcher.Stop(shutdownCtx); err != nil {
		slog.Error("stop config watcher", "err", err)
	}

	// Stop PKI manager.
	if err := pkiManager.Stop(shutdownCtx); err != nil {
		slog.Error("stop pki manager", "err", err)
	}

	// Stop HTTP server.
	if err := httpSrv.Stop(shutdownCtx); err != nil {
		slog.Error("stop http server", "err", err)
	}

	// Stop metrics server.
	if err := metricsSrv.Stop(shutdownCtx); err != nil {
		slog.Error("stop metrics server", "err", err)
	}

	return nil
}

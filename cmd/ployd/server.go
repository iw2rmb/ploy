package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/server/handlers"
	"github.com/iw2rmb/ploy/internal/server/pki"
	"github.com/iw2rmb/ploy/internal/server/recovery"
	"github.com/iw2rmb/ploy/internal/server/scheduler"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/store/batchscheduler"
	"github.com/iw2rmb/ploy/internal/store/ttlworker"
)

// run executes the main server loop and blocks until the context is canceled.
func run(ctx context.Context, cfg config.Config, st store.Store, authorizer *auth.Authorizer, tokenSecret string, bs blobstore.Store, bp *blobpersist.Service) error {
	// Initialize PKI manager for certificate renewal.
	rotator := pki.NewDefaultRotator(slog.Default())
	pkiManager, err := pki.New(pki.Options{
		Config:  cfg.PKI,
		Rotator: rotator,
	})
	if err != nil {
		return fmt.Errorf("create pki manager: %w", err)
	}

	// Initialize TTL worker.
	var ttlWorker *ttlworker.Worker
	if st != nil {
		ttlWorker, err = ttlworker.New(ttlworker.Options{
			Store:          st,
			TTL:            cfg.Scheduler.TTL,
			Interval:       cfg.Scheduler.TTLInterval,
			Logger:         slog.Default(),
			DropPartitions: cfg.Scheduler.DropPartitions,
		})
		if err != nil {
			return fmt.Errorf("create ttl worker: %w", err)
		}
	}

	// Initialize events service for SSE fanout.
	eventsService, err := server.NewEventsService(server.EventsOptions{
		BufferSize:  32,
		HistorySize: 256,
		Logger:      slog.Default(),
		Store:       st,
	})
	if err != nil {
		return fmt.Errorf("create events service: %w", err)
	}

	// Initialize batch scheduler for processing pending repos in batch runs.
	// The scheduler is disabled when BatchSchedulerInterval is 0.
	var batchSched *batchscheduler.Scheduler
	if cfg.Scheduler.BatchSchedulerInterval > 0 {
		repoStarter := handlers.NewBatchRepoStarter(st)
		batchSched, err = batchscheduler.New(batchscheduler.Options{
			Store:       st,
			RepoStarter: repoStarter,
			Interval:    cfg.Scheduler.BatchSchedulerInterval,
			Logger:      slog.Default(),
		})
		if err != nil {
			return fmt.Errorf("create batch scheduler: %w", err)
		}
	}

	// Initialize stale running-job recovery task.
	// The task is disabled when StaleJobRecoveryInterval is explicitly set to 0.
	var staleRecoveryTask *recovery.StaleJobRecoveryTask
	if st != nil && cfg.Scheduler.StaleJobRecoveryInterval != 0 {
		staleRecoveryTask, err = recovery.NewStaleJobRecoveryTask(recovery.Options{
			Store:          st,
			EventsService:  eventsService,
			Interval:       cfg.Scheduler.StaleJobRecoveryInterval,
			NodeStaleAfter: cfg.Scheduler.NodeStaleAfter,
			Logger:         slog.Default(),
		})
		if err != nil {
			return fmt.Errorf("create stale job recovery task: %w", err)
		}
	}

	// Initialize scheduler and register background tasks.
	sched := scheduler.New()
	if ttlWorker != nil {
		sched.AddTask(ttlWorker)
	}
	if batchSched != nil {
		sched.AddTask(batchSched)
	}
	if staleRecoveryTask != nil {
		sched.AddTask(staleRecoveryTask)
	}
	// Start PKI manager.
	if err := pkiManager.Start(ctx); err != nil {
		return fmt.Errorf("start pki manager: %w", err)
	}

	// Start scheduler.
	if err := sched.Start(ctx); err != nil {
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start scheduler: %w", err)
	}

	// Initialize HTTP server for API endpoints.
	httpSrv, err := server.NewHTTPServer(server.HTTPOptions{
		Config:     cfg.HTTP,
		Authorizer: authorizer,
	})
	if err != nil {
		return fmt.Errorf("create http server: %w", err)
	}

	// Load global environment variables from the store for ConfigHolder initialization.
	// These are global env entries (CA bundles, Codex auth, API keys) persisted in
	// the config_env table (see docs/envs/README.md#Global Env Configuration).
	var globalEnvEntries []store.ConfigEnv
	if st != nil {
		globalEnvEntries, err = st.ListGlobalEnv(ctx)
		if err != nil {
			// Log but do not fail — global env is additive; server can run without it.
			slog.Warn("failed to load global env from store, continuing with empty map", "err", err)
			globalEnvEntries = nil
		}
	}
	// Convert store entries to ConfigHolder's in-memory multi-target map.
	// Parse target strings from database into typed GlobalEnvTarget values.
	// With composite key (key, target), multiple entries can share the same
	// key with different targets.
	globalEnvMap := globalEnvMapFromStoreEntries(globalEnvEntries)
	slog.Info("loaded global env entries from store", "count", len(globalEnvMap))

	// Server-target consumption: apply server-target entries to process environment on startup.
	applyServerTargetEnv(globalEnvMap)

	// Load global CA entries from the store for ConfigHolder initialization.
	var globalCAEntries []store.ConfigCa
	if st != nil {
		globalCAEntries, err = st.ListConfigCA(ctx)
		if err != nil {
			slog.Warn("failed to load global CA entries from store, continuing with empty set", "err", err)
			globalCAEntries = nil
		}
	}
	slog.Info("loaded global CA entries from store", "count", len(globalCAEntries))

	// Load global home entries from the store for ConfigHolder initialization.
	var globalHomeEntries []store.ConfigHome
	if st != nil {
		globalHomeEntries, err = st.ListConfigHome(ctx)
		if err != nil {
			slog.Warn("failed to load global home entries from store, continuing with empty set", "err", err)
			globalHomeEntries = nil
		}
	}
	slog.Info("loaded global home entries from store", "count", len(globalHomeEntries))

	// Load global in entries from the store for ConfigHolder initialization.
	var globalInEntries []store.ConfigIn
	if st != nil {
		globalInEntries, err = st.ListConfigIn(ctx)
		if err != nil {
			slog.Warn("failed to load global in entries from store, continuing with empty set", "err", err)
			globalInEntries = nil
		}
	}
	slog.Info("loaded global in entries from store", "count", len(globalInEntries))

	// Initialize config holder for runtime configuration access.
	configHolder := handlers.NewConfigHolder(cfg.GitLab, globalEnvMap)

	// Populate ConfigHolder with persisted CA entries keyed by section.
	caBySection := make(map[string][]string)
	for _, e := range globalCAEntries {
		caBySection[e.Section] = append(caBySection[e.Section], e.Hash)
	}
	for section, hashes := range caBySection {
		configHolder.SetConfigCA(section, hashes)
	}

	// Populate ConfigHolder with persisted home entries keyed by section.
	homeBySection := make(map[string][]handlers.ConfigHomeEntry)
	for _, e := range globalHomeEntries {
		homeBySection[e.Section] = append(homeBySection[e.Section], handlers.ConfigHomeEntry{
			Entry:   e.Entry,
			Dst:     e.Dst,
			Section: e.Section,
		})
	}
	for section, entries := range homeBySection {
		configHolder.SetConfigHome(section, entries)
	}

	// Populate ConfigHolder with persisted in entries keyed by section.
	inBySection := make(map[string][]handlers.ConfigInEntry)
	for _, e := range globalInEntries {
		inBySection[e.Section] = append(inBySection[e.Section], handlers.ConfigInEntry{
			Entry:   e.Entry,
			Dst:     e.Dst,
			Section: e.Section,
		})
	}
	for section, entries := range inBySection {
		configHolder.SetConfigIn(section, entries)
	}

	// Execute hard-cut migration: persist rewrite-eligible legacy special env keys
	// as typed ca/home/in records and remove the legacy env records.
	migrationReport := handlers.ScanSpecialEnvKeys(globalEnvMap, caBySection, homeBySection)
	if st != nil {
		execResult, execErr := handlers.ExecuteMigration(ctx, migrationReport, st, configHolder)
		if execErr != nil {
			slog.Error("special env migration: execution failed", "err", execErr)
		} else {
			handlers.LogMigrationExecResult(execResult)
		}
	} else {
		handlers.LogMigrationReport(migrationReport)
	}

	// Register HTTP routes.
	handlers.RegisterRoutes(httpSrv, st, bs, bp, eventsService, configHolder, tokenSecret)

	// Initialize metrics server.
	metricsSrv := server.NewMetricsServer(cfg.Metrics.Listen)

	// Start HTTP server.
	if err := httpSrv.Start(ctx); err != nil {
		// Ensure background tasks are stopped on failure.
		_ = sched.Stop(context.Background())
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start http server: %w", err)
	}

	// Start metrics server.
	if err := metricsSrv.Start(ctx); err != nil {
		// Stop HTTP server on failure to start metrics.
		_ = httpSrv.Stop(context.Background())
		// Stop scheduler to avoid leaking background goroutines.
		_ = sched.Stop(context.Background())
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

// applyServerTargetEnv sets process environment variables for all server-target entries.
// Called once at startup to ensure server-target global env is available to the process.
func applyServerTargetEnv(envMap map[string][]handlers.GlobalEnvVar) {
	for key, entries := range envMap {
		for _, e := range entries {
			if e.Target == domaintypes.GlobalEnvTargetServer {
				if err := os.Setenv(key, e.Value); err != nil {
					slog.Warn("failed to set server-target env var", "key", key, "err", err)
				}
			}
		}
	}
}

func globalEnvMapFromStoreEntries(entries []store.ConfigEnv) map[string][]handlers.GlobalEnvVar {
	globalEnvMap := make(map[string][]handlers.GlobalEnvVar)
	for _, e := range entries {
		// Parse target from database; empty or invalid targets are dropped.
		target, err := domaintypes.ParseGlobalEnvTarget(e.Target)
		if err != nil {
			slog.Warn("invalid target in stored global env, dropping entry",
				"key", e.Key,
				"target", e.Target,
				"err", err,
			)
			continue
		}
		globalEnvMap[e.Key] = append(globalEnvMap[e.Key], handlers.GlobalEnvVar{
			Value:  e.Value,
			Target: target,
			Secret: e.Secret,
		})
	}
	return globalEnvMap
}

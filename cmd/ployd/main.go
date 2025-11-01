package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

func main() {
	// Allow env to supply the default config path; CLI flag still has highest precedence.
	defaultConfigPath := strings.TrimSpace(os.Getenv("PLOYD_CONFIG_PATH"))
	if defaultConfigPath == "" {
		defaultConfigPath = "/etc/ploy/ployd.yaml"
	}

	var configPath string
	flag.StringVar(&configPath, "config", defaultConfigPath, "Path to ployd configuration (flag overrides $PLOYD_CONFIG_PATH)")
	flag.Parse()

	// Configure structured logger early (will be reconfigured after loading config).
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{})))

	// Load configuration from file.
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("load config", "err", err, "path", configPath)
		os.Exit(1)
	}

	// Reconfigure logger based on config.
	if err := initLogging(cfg.Logging); err != nil {
		slog.Error("initialize logging", "err", err)
		os.Exit(1)
	}

	// Resolve PostgreSQL DSN from environment or config.
	dsn := resolvePgDSN(cfg)
	if dsn == "" {
		slog.Error("postgresql dsn not configured", "hint", "set PLOY_SERVER_PG_DSN or configure postgres.dsn in config file")
		os.Exit(1)
	}

	// Initialize store.
	ctx := context.Background()
	st, err := store.NewStore(ctx, dsn)
	if err != nil {
		slog.Error("initialize store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	// Initialize Authorizer for mTLS-based authentication.
	// Default role is RoleControlPlane; AllowInsecure is false for production.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
		DefaultRole:   auth.RoleControlPlane,
	})

	// Reflect configured transport settings in startup logs (before listeners come up).
	slog.Info("ployd server starting",
		"config", configPath,
		"tls", cfg.HTTP.TLS.Enabled,
		"mtls", cfg.HTTP.TLS.RequireClientCert,
	)

	// Set up signal handling for graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run server.
	if err := run(ctx, cfg, configPath, st, authorizer); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}

	slog.Info("ployd server stopped")
}

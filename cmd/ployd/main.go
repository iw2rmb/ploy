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

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
	iversion "github.com/iw2rmb/ploy/internal/version"
)

func main() {
	// Allow env to supply the default config path; CLI flag still has highest precedence.
	defaultConfigPath := strings.TrimSpace(os.Getenv("PLOYD_CONFIG_PATH"))
	if defaultConfigPath == "" {
		defaultConfigPath = "/etc/ploy/ployd.yaml"
	}

	var configPath string
	var showVersion bool
	flag.StringVar(&configPath, "config", defaultConfigPath, "Path to ployd configuration (flag overrides $PLOYD_CONFIG_PATH)")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.Parse()

	if showVersion {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})))
		slog.Info("ployd", "version", iversion.Version, "commit", iversion.Commit, "built_at", iversion.BuiltAt)
		return
	}

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
		slog.Error("postgresql dsn not configured", "hint", "set PLOY_POSTGRES_DSN or configure postgres.dsn in config file")
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

	// Run database migrations to ensure schema is present and up-to-date.
	if err := store.RunMigrations(ctx, st.Pool()); err != nil {
		slog.Error("run migrations", "err", err)
		os.Exit(1)
	}

	// No cluster table initialization; cluster-id is provided via environment.

	// Load auth secret from env or config
	authSecret := os.Getenv("PLOY_AUTH_SECRET")
	if authSecret == "" && cfg.Auth.BearerTokens.Secret != "" {
		authSecret = cfg.Auth.BearerTokens.Secret
	}
	if authSecret == "" {
		slog.Error("PLOY_AUTH_SECRET environment variable or auth.bearer_tokens.secret config required")
		os.Exit(1)
	}

	// Initialize Authorizer with bearer token support.
	// Default role is RoleControlPlane; AllowInsecure is false for production.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
		DefaultRole:   auth.RoleControlPlane,
		TokenSecret:   authSecret,
		Querier:       st,
	})

	// Reflect configured transport settings in startup logs (before listeners come up).
	slog.Info("ployd server starting",
		"config", configPath,
		"bearer_tokens", cfg.Auth.BearerTokens.Enabled,
	)

	// Set up signal handling for graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run server.
	if err := run(ctx, cfg, configPath, st, authorizer, authSecret); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}

	slog.Info("ployd server stopped")
}

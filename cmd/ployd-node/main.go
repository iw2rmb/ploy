//go:build !stub

package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/iw2rmb/ploy/internal/nodeagent"
	iversion "github.com/iw2rmb/ploy/internal/version"
)

// indirection points for testability.
var (
	loadConfig = nodeagent.LoadConfig
	newAgent   = func(cfg nodeagent.Config) (interface{ Run(context.Context) error }, error) {
		return nodeagent.New(cfg)
	}
)

func main() {
	os.Exit(run())
}

func run() int {
	var configPath string
	var showVersion bool
	flag.StringVar(&configPath, "config", "/etc/ploy/ployd-node.yaml", "Path to ployd-node configuration")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.Parse()

	// Configure structured logger early.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{})))

	if showVersion {
		slog.Info("ployd-node", "version", iversion.Version, "commit", iversion.Commit, "built_at", iversion.BuiltAt)
		return 0
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		return 1
	}

	agent, err := newAgent(cfg)
	if err != nil {
		slog.Error("initialise node agent", "err", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := agent.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("node agent exited", "err", err)
		return 1
	}
	return 0
}

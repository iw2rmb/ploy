//go:build !stub
// +build !stub

package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/iw2rmb/ploy/internal/nodeagent"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "/etc/ploy/ployd-node.yaml", "Path to ployd-node configuration")
	flag.Parse()

	// Configure structured logger early.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{})))

	cfg, err := nodeagent.LoadConfig(configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	agent, err := nodeagent.New(cfg)
	if err != nil {
		slog.Error("initialise node agent", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := agent.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("node agent exited", "err", err)
		os.Exit(1)
	}
}

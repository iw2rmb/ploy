package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/iw2rmb/ploy/internal/nodeagent"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "/etc/ploy/ployd-node.yaml", "Path to ployd-node configuration")
	flag.Parse()

	cfg, err := nodeagent.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	agent, err := nodeagent.New(cfg)
	if err != nil {
		log.Fatalf("initialise node agent: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := agent.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("node agent exited: %v", err)
	}
}

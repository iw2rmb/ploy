package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iw2rmb/ploy/internal/ployd/config"
	"github.com/iw2rmb/ploy/internal/ployd/daemon"
)

func main() {
	var (
		configPath   string
		overrideMode string
	)
	flag.StringVar(&configPath, "config", "/etc/ploy/ployd.yaml", "Path to ployd configuration")
	flag.StringVar(&overrideMode, "mode", "", "Override daemon mode (bootstrap, worker, beacon)")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if overrideMode != "" {
		cfg.Mode = overrideMode
	}

	svc, err := daemon.NewDefault(cfg)
	if err != nil {
		log.Fatalf("initialise daemon: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-hup:
				reloadCtx, cancelReload := context.WithTimeout(context.Background(), 10*time.Second)
				updated, err := config.Load(configPath)
				if err != nil {
					log.Printf("reload config: %v", err)
					cancelReload()
					continue
				}
				if overrideMode != "" {
					updated.Mode = overrideMode
				}
				if err := svc.Reload(reloadCtx, updated); err != nil {
					log.Printf("reload daemon: %v", err)
				}
				cancelReload()
			}
		}
	}()

	if err := svc.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("daemon exited: %v", err)
	}
}

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/daemon"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "bootstrap-ca" {
		if err := runBootstrapCA(os.Args[2:]); err != nil {
			log.Fatalf("bootstrap-ca: %v", err)
		}
		return
	}
	var configPath string
	flag.StringVar(&configPath, "config", "/etc/ploy/ployd.yaml", "Path to ployd configuration")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
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

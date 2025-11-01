package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/daemon"
	"github.com/iw2rmb/ploy/internal/node/lifecycle"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "bootstrap-ca":
			if err := runBootstrapCA(os.Args[2:]); err != nil {
				log.Fatalf("bootstrap-ca: %v", err)
			}
			return
		case "slot-guard":
			if err := runSlotGuard(os.Args[2:]); err != nil {
				log.Fatalf("slot-guard: %v", err)
			}
			return
		case "status-snapshot":
			if err := runStatusSnapshot(os.Args[2:]); err != nil {
				log.Fatalf("status-snapshot: %v", err)
			}
			return
		}
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

// runStatusSnapshot collects a one-shot lifecycle snapshot and prints it as JSON.
func runStatusSnapshot(args []string) error {
	fs := flag.NewFlagSet("status-snapshot", flag.ContinueOnError)
	role := fs.String("role", "worker", "Node role")
	nodeID := fs.String("node-id", "", "Node identifier")
	samples := fs.Int("samples", 1, "Number of samples to collect")
	interval := fs.Duration("interval", 2*time.Second, "Interval between samples")
	_ = fs.Parse(args)

	if *samples < 1 {
		*samples = 1
	}

	collector := lifecycle.NewCollector(lifecycle.Options{
		Role:   *role,
		NodeID: *nodeID,
	})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	for i := 0; i < *samples; i++ {
		snap, err := collector.Collect(context.Background())
		if err != nil {
			return err
		}
		if err := enc.Encode(snap.Status); err != nil {
			return err
		}
		if i+1 < *samples {
			time.Sleep(*interval)
		}
	}
	return nil
}

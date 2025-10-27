package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
	"github.com/iw2rmb/ploy/internal/etcdutil"
)

func runSlotGuard(args []string) error {
	fs := flag.NewFlagSet("slot-guard", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	slotID := fs.String("slot", "", "Slot identifier to guard")
	configPath := fs.String("config", "/etc/ploy/ployd.yaml", "Path to ployd configuration file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*slotID) == "" {
		return errors.New("--slot is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	etcdCfg, err := etcdutil.ConfigFromEnv()
	if err != nil {
		return fmt.Errorf("etcd config: %w", err)
	}
	client, err := clientv3.New(etcdCfg)
	if err != nil {
		return fmt.Errorf("connect etcd: %w", err)
	}
	defer client.Close()

	clusterID := determineClusterID(cfg)
	slotStore, err := transfers.NewSlotStore(client, transfers.SlotStoreOptions{
		ClusterID: clusterID,
	})
	if err != nil {
		return fmt.Errorf("slot store: %w", err)
	}
	defer slotStore.Close()

	guard, err := transfers.NewGuard(transfers.GuardOptions{
		Logger:     log.New(os.Stderr, "slot-guard: ", log.LstdFlags),
		SlotID:     strings.TrimSpace(*slotID),
		Store:      slotStore,
		BaseDir:    cfg.Transfers.BaseDir,
		ServerPath: cfg.Transfers.GuardBinary,
		Clock:      time.Now,
	})
	if err != nil {
		return err
	}
	return guard.Run(context.Background())
}

func determineClusterID(cfg config.Config) string {
	if cfg.Metadata != nil {
		if value, ok := cfg.Metadata["cluster_id"]; ok {
			if str, ok := value.(string); ok {
				if trimmed := strings.TrimSpace(str); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	if env := strings.TrimSpace(os.Getenv("PLOY_CLUSTER_ID")); env != "" {
		return env
	}
	if data, err := os.ReadFile("/etc/ploy/cluster-id"); err == nil {
		if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
			return trimmed
		}
	}
	return "default"
}

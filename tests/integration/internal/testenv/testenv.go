//go:build integration
// +build integration

package testenv

import (
	"context"
	"testing"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"

	"github.com/iw2rmb/ploy/internal/storage"
	seaweedfs "github.com/iw2rmb/ploy/internal/storage/providers/seaweedfs"
	"github.com/iw2rmb/ploy/internal/testing/helpers"
)

func RequireNomadClient(t *testing.T) *nomadapi.Client {
	t.Helper()

	addr := helpers.GetEnvOrDefault("NOMAD_ADDR", "http://localhost:4646")
	cfg := nomadapi.DefaultConfig()
	cfg.Address = addr

	client, err := nomadapi.NewClient(cfg)
	if err != nil {
		t.Skipf("Nomad unavailable at %s: %v", addr, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Skipf("Nomad leader not reachable at %s", addr)
		default:
			leader, err := client.Status().Leader()
			if err == nil && leader != "" {
				return client
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func RequireConsulClient(t *testing.T) *consulapi.Client {
	t.Helper()

	addr := helpers.GetEnvOrDefault("CONSUL_HTTP_ADDR", "localhost:8500")
	cfg := consulapi.DefaultConfig()
	cfg.Address = addr

	client, err := consulapi.NewClient(cfg)
	if err != nil {
		t.Skipf("Consul unavailable at %s: %v", addr, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Skipf("Consul leader not reachable at %s", addr)
		default:
			if leader, err := client.Status().Leader(); err == nil && leader != "" {
				return client
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func RequireSeaweedStorage(t *testing.T) storage.Storage {
	t.Helper()

	master := helpers.GetEnvOrDefault("SEAWEEDFS_MASTER", "localhost:9333")
	filer := helpers.GetEnvOrDefault("SEAWEEDFS_FILER", "localhost:8888")
	collection := helpers.GetEnvOrDefault("SEAWEEDFS_COLLECTION", "artifacts")

	provider, err := seaweedfs.New(seaweedfs.Config{
		Master:     master,
		Filer:      filer,
		Collection: collection,
		Timeout:    15,
	})
	if err != nil {
		t.Skipf("SeaweedFS provider init failed (master=%s filer=%s): %v", master, filer, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := provider.Health(ctx); err != nil {
		t.Skipf("SeaweedFS health check failed: %v", err)
	}

	return provider
}

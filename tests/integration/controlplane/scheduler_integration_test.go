//go:build integration

package controlplane_test

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

func TestIntegrationSchedulerSingleClaim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	etcd, client := startIntegrationEtcd(t)
	defer etcd.Close()
	defer client.Close()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 2 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer sched.Close()

	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-int-1",
		StepID:      "plan",
		Priority:    "default",
		MaxAttempts: 2,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	var (
		wg     sync.WaitGroup
		succMu sync.Mutex
		succ   []*scheduler.ClaimResult
	)
	workers := []string{"node-1", "node-2", "node-3"}
	wg.Add(len(workers))
	for _, nodeID := range workers {
		nodeID := nodeID
		go func() {
			defer wg.Done()
			res, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: nodeID})
			if err != nil {
				return
			}
			succMu.Lock()
			succ = append(succ, res)
			succMu.Unlock()
		}()
	}
	wg.Wait()

	if len(succ) != 1 {
		t.Fatalf("expected single claim, got %d", len(succ))
	}
	if succ[0].Job.ID != job.ID {
		t.Fatalf("claimed job mismatch: %s", succ[0].Job.ID)
	}

	// allow lease to expire
	time.Sleep(2500 * time.Millisecond)

	// wait for requeue and claim again
	var second *scheduler.ClaimResult
	for start := time.Now(); time.Since(start) < 5*time.Second; {
		res, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: "node-4"})
		if err == nil {
			second = res
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if second == nil {
		t.Fatalf("job did not requeue after lease expiry")
	}
}

func startIntegrationEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir
	clientURL := mustParseURL("http://127.0.0.1:0")
	peerURL := mustParseURL("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "integration"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "integration-scheduler"
	cfg.LogLevel = "panic"
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{filepath.Join(dir, "etcd.log")}

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start etcd: %v", err)
	}
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Server.Stop()
		t.Fatalf("etcd start timeout")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		t.Fatalf("client: %v", err)
	}

	return e, client
}

func mustParseURL(raw string) url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *parsed
}

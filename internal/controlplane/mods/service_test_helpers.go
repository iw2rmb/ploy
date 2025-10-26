package mods

import (
	"context"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/google/uuid"
)

// newTestService wires a Service backed by the embedded etcd fixture and fake scheduler.
func newTestService(t *testing.T, client *clientv3.Client, scheduler *fakeScheduler) *Service {
	t.Helper()
	service, err := NewService(client, Options{
		Prefix:    "mods/",
		Scheduler: scheduler,
		Clock:     func() time.Time { return time.Unix(0, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

// newTestEtcd spins up an embedded etcd for control-plane tests.
func newTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir
	clientURL := mustURL(t, "http://127.0.0.1:0")
	peerURL := mustURL(t, "http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "mods-control-plane"
	cfg.InitialCluster = cfg.Name + "=" + peerURL.String()
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{filepath.Join(dir, "etcd.log")}
	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start etcd: %v", err)
	}
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for etcd ready")
	}
	endpoint := e.Clients[0].Addr().String()
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new etcd client: %v", err)
	}
	return e, client
}

// mustURL parses a URL literal or fails the test immediately.
func mustURL(t *testing.T, raw string) url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return *parsed
}

type fakeScheduler struct {
	mu   sync.Mutex
	jobs []StageJobSpec
}

// newFakeScheduler returns a concurrency-safe stub scheduler.
func newFakeScheduler() *fakeScheduler {
	return &fakeScheduler{jobs: make([]StageJobSpec, 0)}
}

// SubmitStageJob records the scheduled job without touching real infrastructure.
func (s *fakeScheduler) SubmitStageJob(ctx context.Context, spec StageJobSpec) (StageJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := StageJob{
		JobID:    spec.JobID,
		TicketID: spec.TicketID,
		StageID:  spec.StageID,
	}
	if job.JobID == "" {
		job.JobID = uuid.NewString()
	}
	spec.JobID = job.JobID
	s.jobs = append(s.jobs, spec)
	return job, nil
}

// SubmittedJobs returns a snapshot copy of recorded job specs.
func (s *fakeScheduler) SubmittedJobs() []StageJobSpec {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StageJobSpec, len(s.jobs))
	copy(out, s.jobs)
	return out
}

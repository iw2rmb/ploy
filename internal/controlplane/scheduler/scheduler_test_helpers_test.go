package scheduler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

func mustLoadJobDocument(t *testing.T, ctx context.Context, client *clientv3.Client, key string) map[string]any {
	t.Helper()
	doc, err := loadJobDocument(ctx, client, key)
	if err != nil {
		t.Fatalf("load job document: %v", err)
	}
	return doc
}

func loadJobDocument(ctx context.Context, client *clientv3.Client, key string) (map[string]any, error) {
	resp, err := client.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("job key %s not found", key)
	}
	var doc map[string]any
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func newScheduler(t *testing.T, client *clientv3.Client) *scheduler.Scheduler {
	t.Helper()
	return mustNewScheduler(t, client, defaultOptions())
}

func mustNewScheduler(t *testing.T, client *clientv3.Client, opts scheduler.Options) *scheduler.Scheduler {
	t.Helper()
	sched, err := scheduler.New(client, opts)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	return sched
}

func defaultOptions() scheduler.Options {
	return scheduler.Options{
		JobsPrefix:      "mods/",
		QueuePrefix:     "queue/mods/",
		LeasesPrefix:    "leases/jobs/",
		NodesPrefix:     "nodes/",
		GCPrefix:        "gc/jobs/",
		LeaseTTL:        3 * time.Second,
		ClockSkewBuffer: 500 * time.Millisecond,
	}
}

func newTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
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
	cfg.Name = "default"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "scheduler-test"
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

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func gaugeValue(reg *prometheus.Registry, name string, labels map[string]string) (float64, bool) {
	metric, ok := gatherMetric(reg, name, labels)
	if !ok || metric.GetGauge() == nil {
		return 0, false
	}
	return metric.GetGauge().GetValue(), true
}

func counterValue(reg *prometheus.Registry, name string, labels map[string]string) (float64, bool) {
	metric, ok := gatherMetric(reg, name, labels)
	if !ok || metric.GetCounter() == nil {
		return 0, false
	}
	return metric.GetCounter().GetValue(), true
}

func histogramValue(reg *prometheus.Registry, name string, labels map[string]string) (uint64, float64, bool) {
	metric, ok := gatherMetric(reg, name, labels)
	if !ok || metric.GetHistogram() == nil {
		return 0, 0, false
	}
	hist := metric.GetHistogram()
	return hist.GetSampleCount(), hist.GetSampleSum(), true
}

func gatherMetric(reg *prometheus.Registry, name string, labels map[string]string) (*dto.Metric, bool) {
	mfs, err := reg.Gather()
	if err != nil {
		return nil, false
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, metric := range mf.GetMetric() {
			if labelsMatch(metric, labels) {
				return metric, true
			}
		}
	}
	return nil, false
}

func labelsMatch(metric *dto.Metric, want map[string]string) bool {
	if len(want) == 0 {
		return true
	}
	for key, value := range want {
		found := false
		for _, pair := range metric.GetLabel() {
			if pair.GetName() == key && pair.GetValue() == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

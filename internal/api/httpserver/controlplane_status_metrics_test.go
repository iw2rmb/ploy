package httpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/api/httpserver/security"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/metrics"
)

func TestStatusSummaryIncludesQueueAndWorkers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	_ = mustBootstrapCluster(t, client, "cluster-alpha")

	registryClient, err := registry.NewWorkerRegistry(client, "cluster-alpha")
	if err != nil {
		t.Fatalf("new worker registry: %v", err)
	}
	now := time.Date(2025, 10, 24, 9, 45, 0, 0, time.UTC)
	descriptor := registry.WorkerDescriptor{
		ID:           "worker-ready",
		Address:      "10.21.0.10",
		RegisteredAt: now,
		Status: registry.WorkerStatus{
			Phase:     registry.WorkerPhaseReady,
			CheckedAt: now,
		},
	}
	if _, err := registryClient.Register(ctx, descriptor); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	promRegistry := prometheus.NewRegistry()
	recorder, err := metrics.NewSchedulerMetrics(promRegistry)
	if err != nil {
		t.Fatalf("new scheduler metrics: %v", err)
	}

	sched, err := scheduler.New(client, scheduler.Options{
		LeaseTTL: 3 * time.Second,
		Metrics:  recorder,
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	principal := newTestPrincipal([]string{security.ScopeAdmin})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
		Gatherer:  promRegistry,
		Auth:      security.NewManager(&testTokenVerifier{principal: principal}),
	})

	if _, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-status",
		StepID:      "plan",
		Priority:    "default",
		MaxAttempts: 1,
	}); err != nil {
		t.Fatalf("submit job: %v", err)
	}

	req := newMTLSRequest(t, http.MethodGet, "/v1/status?cluster_id=cluster-alpha", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from status endpoint, got %d", rec.Code)
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", cache)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if ts, _ := body["timestamp"].(string); strings.TrimSpace(ts) == "" {
		t.Fatalf("expected timestamp in status response")
	}
	queueBlock, ok := body["queue"].(map[string]any)
	if !ok {
		t.Fatalf("expected queue block in status response")
	}
	totalDepth, _ := queueBlock["total_depth"].(float64)
	if totalDepth < 1 {
		t.Fatalf("expected positive total_depth, got %v", totalDepth)
	}
	priorities, ok := queueBlock["priorities"].([]any)
	if !ok || len(priorities) == 0 {
		t.Fatalf("expected queue priorities slice, got %#v", queueBlock["priorities"])
	}
	workersBlock, ok := body["workers"].(map[string]any)
	if !ok {
		t.Fatalf("expected workers block in status response")
	}
	totalWorkers, _ := workersBlock["total"].(float64)
	if totalWorkers != 1 {
		t.Fatalf("expected total workers 1, got %v", totalWorkers)
	}
	phases, _ := workersBlock["phases"].(map[string]any)
	if ready, _ := phases[registry.WorkerPhaseReady].(float64); ready != 1 {
		t.Fatalf("expected ready workers 1, got %v", ready)
	}
}

func TestMetricsEndpointExposesPrometheus(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	reg := prometheus.NewRegistry()
	recorder, err := metrics.NewSchedulerMetrics(reg)
	if err != nil {
		t.Fatalf("new scheduler metrics: %v", err)
	}

	sched, err := scheduler.New(client, scheduler.Options{
		LeaseTTL: 3 * time.Second,
		Metrics:  recorder,
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	server := httptest.NewServer(newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Gatherer:  reg,
		Etcd:      client,
	}))
	defer server.Close()

	postJSON(t, server.URL+"/v1/jobs", map[string]any{
		"ticket":       "mod-observe",
		"step_id":      "build",
		"priority":     "default",
		"max_attempts": 1,
	})

	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("fetch metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "ploy_controlplane_queue_depth") {
		t.Fatalf("expected queue depth metric in scrape output")
	}
	if !strings.Contains(text, `priority="default"`) {
		t.Fatalf("expected queue depth labels recorded")
	}
}

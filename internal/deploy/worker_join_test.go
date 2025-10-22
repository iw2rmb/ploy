package deploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/registry"
)

func TestWorkerJoin(t *testing.T) {
	t.Helper()
	t.Run("success registers worker and stores certificate", func(t *testing.T) {
		ctx := context.Background()
		etcd, client := newTestEtcd(t)
		defer etcd.Close()
		defer func() {
			_ = client.Close()
		}()

		manager := mustNewCARotationManager(t, client, "cluster-alpha")
		_, err := manager.Bootstrap(ctx, BootstrapOptions{
			BeaconIDs: []string{"beacon-main"},
		})
		if err != nil {
			t.Fatalf("bootstrap: %v", err)
		}

		opt := WorkerJoinOptions{
			ClusterID: "cluster-alpha",
			WorkerID:  "worker-01",
			Address:   "10.20.1.15",
			Labels: map[string]string{
				"role": "build",
			},
			HealthProbes: []WorkerHealthProbe{
				{Name: "ready", Endpoint: "http://worker-01.local/healthz"},
			},
			HealthChecker: fakeHealthChecker{
				results: []registry.WorkerProbeResult{
					{
						Name:      "ready",
						Endpoint:  "http://worker-01.local/healthz",
						Passed:    true,
						CheckedAt: time.Date(2025, 10, 22, 12, 0, 0, 0, time.UTC),
					},
				},
			},
			Clock: func() time.Time {
				return time.Date(2025, 10, 22, 11, 58, 0, 0, time.UTC)
			},
		}

		result, err := RunWorkerJoin(ctx, client, opt)
		if err != nil {
			t.Fatalf("RunWorkerJoin returned error: %v", err)
		}

		if result.Descriptor.ID != "worker-01" {
			t.Fatalf("expected descriptor ID worker-01, got %s", result.Descriptor.ID)
		}
		if result.Descriptor.Status.Phase != registry.WorkerPhaseReady {
			t.Fatalf("expected status phase ready, got %s", result.Descriptor.Status.Phase)
		}
		if result.Descriptor.CertificateVersion == "" {
			t.Fatalf("expected certificate version recorded")
		}
		if result.Certificate.NodeID != "worker-01" {
			t.Fatalf("expected certificate for worker-01, got %s", result.Certificate.NodeID)
		}
		if len(result.Health) != 1 || !result.Health[0].Passed {
			t.Fatalf("expected health probe to pass, got %+v", result.Health)
		}

		state, err := manager.State(ctx)
		if err != nil {
			t.Fatalf("State returned error: %v", err)
		}
		if len(state.Nodes.Workers) != 1 || state.Nodes.Workers[0] != "worker-01" {
			t.Fatalf("expected worker inventory to include worker-01, got %v", state.Nodes.Workers)
		}
		if _, ok := state.WorkerCertificates["worker-01"]; !ok {
			t.Fatalf("expected worker certificate persisted")
		}

		reg, err := registry.NewWorkerRegistry(client, "cluster-alpha")
		if err != nil {
			t.Fatalf("NewWorkerRegistry: %v", err)
		}
		record, err := reg.Get(ctx, "worker-01")
		if err != nil {
			t.Fatalf("registry get: %v", err)
		}
		if record.Descriptor.Status.Phase != registry.WorkerPhaseReady {
			t.Fatalf("expected registry status ready, got %s", record.Descriptor.Status.Phase)
		}
	})

	t.Run("health failure rolls back metadata", func(t *testing.T) {
		ctx := context.Background()
		etcd, client := newTestEtcd(t)
		defer etcd.Close()
		defer func() {
			_ = client.Close()
		}()

		manager := mustNewCARotationManager(t, client, "cluster-beta")
		_, err := manager.Bootstrap(ctx, BootstrapOptions{
			BeaconIDs: []string{"beacon-main"},
		})
		if err != nil {
			t.Fatalf("bootstrap: %v", err)
		}

		opt := WorkerJoinOptions{
			ClusterID: "cluster-beta",
			WorkerID:  "worker-bad",
			Address:   "10.20.1.25",
			HealthProbes: []WorkerHealthProbe{
				{Name: "ready", Endpoint: "http://worker-bad.local/healthz"},
			},
			HealthChecker: fakeHealthChecker{
				results: []registry.WorkerProbeResult{
					{
						Name:      "ready",
						Endpoint:  "http://worker-bad.local/healthz",
						Passed:    false,
						Message:   "probe failed",
						CheckedAt: time.Date(2025, 10, 22, 13, 0, 0, 0, time.UTC),
					},
				},
			},
		}

		_, err = RunWorkerJoin(ctx, client, opt)
		if err == nil {
			t.Fatalf("expected RunWorkerJoin to fail when health probe fails")
		}

		state, err := manager.State(ctx)
		if err != nil {
			t.Fatalf("State returned error: %v", err)
		}
		if len(state.Nodes.Workers) != 0 {
			t.Fatalf("expected no workers recorded after rollback, got %v", state.Nodes.Workers)
		}
		if _, ok := state.WorkerCertificates["worker-bad"]; ok {
			t.Fatalf("expected worker certificate removed after failure")
		}

		reg, err := registry.NewWorkerRegistry(client, "cluster-beta")
		if err != nil {
			t.Fatalf("NewWorkerRegistry: %v", err)
		}
		_, err = reg.Get(ctx, "worker-bad")
		if !errors.Is(err, registry.ErrWorkerNotFound) {
			t.Fatalf("expected ErrWorkerNotFound after rollback, got %v", err)
		}
	})

	t.Run("dry run does not mutate state", func(t *testing.T) {
		ctx := context.Background()
		etcd, client := newTestEtcd(t)
		defer etcd.Close()
		defer func() {
			_ = client.Close()
		}()

		manager := mustNewCARotationManager(t, client, "cluster-gamma")
		_, err := manager.Bootstrap(ctx, BootstrapOptions{
			BeaconIDs: []string{"beacon-main"},
		})
		if err != nil {
			t.Fatalf("bootstrap: %v", err)
		}

		opt := WorkerJoinOptions{
			ClusterID:    "cluster-gamma",
			WorkerID:     "worker-preview",
			Address:      "10.20.1.45",
			HealthProbes: []WorkerHealthProbe{{Name: "ready", Endpoint: "http://worker-preview.local/healthz"}},
			HealthChecker: fakeHealthChecker{
				results: []registry.WorkerProbeResult{
					{
						Name:      "ready",
						Endpoint:  "http://worker-preview.local/healthz",
						Passed:    true,
						CheckedAt: time.Date(2025, 10, 22, 13, 30, 0, 0, time.UTC),
					},
				},
			},
			DryRun: true,
		}

		result, err := RunWorkerJoin(ctx, client, opt)
		if err != nil {
			t.Fatalf("RunWorkerJoin dry run error: %v", err)
		}
		if !result.DryRun {
			t.Fatalf("expected dry run flag in result")
		}

		state, err := manager.State(ctx)
		if err != nil {
			t.Fatalf("State returned error: %v", err)
		}
		if len(state.Nodes.Workers) != 0 {
			t.Fatalf("expected dry run to leave worker list empty, got %v", state.Nodes.Workers)
		}
		if len(state.WorkerCertificates) != 0 {
			t.Fatalf("expected dry run to avoid issuing certificates")
		}

		reg, err := registry.NewWorkerRegistry(client, "cluster-gamma")
		if err != nil {
			t.Fatalf("NewWorkerRegistry: %v", err)
		}
		_, err = reg.Get(ctx, "worker-preview")
		if !errors.Is(err, registry.ErrWorkerNotFound) {
			t.Fatalf("expected ErrWorkerNotFound after dry run, got %v", err)
		}
	})
}

type fakeHealthChecker struct {
	results []registry.WorkerProbeResult
	err     error
}

func (f fakeHealthChecker) Run(_ context.Context, workerID string, address string, probes []WorkerHealthProbe) ([]registry.WorkerProbeResult, error) {
	return f.results, f.err
}

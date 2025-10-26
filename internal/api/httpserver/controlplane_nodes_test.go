package httpserver_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestServerNodesLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	manager := mustBootstrapCluster(t, client, "cluster-alpha")

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	server := httptest.NewServer(newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	status, nodeResp := postJSONStatus(t, server.URL+"/v1/nodes", map[string]any{
		"cluster_id": "cluster-alpha",
		"address":    "10.20.1.50",
		"labels": map[string]any{
			"role": "build",
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", status)
	}

	workerID, ok := nodeResp["worker_id"].(string)
	if !ok || strings.TrimSpace(workerID) == "" {
		t.Fatalf("expected worker_id in response, got %+v", nodeResp["worker_id"])
	}

	desc, ok := nodeResp["descriptor"].(map[string]any)
	if !ok {
		t.Fatalf("expected descriptor map in response")
	}
	if address, _ := desc["address"].(string); address != "10.20.1.50" {
		t.Fatalf("unexpected descriptor address %q", address)
	}
	statusMap, ok := desc["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected status block in descriptor")
	}
	if phase, _ := statusMap["phase"].(string); phase != registry.WorkerPhaseReady {
		t.Fatalf("expected ready phase, got %q", phase)
	}

	listStatus, listResp := getJSONStatus(t, fmt.Sprintf("%s/v1/nodes?cluster_id=%s", server.URL, url.QueryEscape("cluster-alpha")))
	if listStatus != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listStatus)
	}
	nodes, ok := listResp["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		t.Fatalf("expected nodes array in list response")
	}
	var entry map[string]any
	for _, item := range nodes {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := m["id"].(string); id == workerID {
			entry = m
			break
		}
	}
	if entry == nil {
		t.Fatalf("worker %s missing from listing", workerID)
	}
	if version, _ := entry["certificate_version"].(string); strings.TrimSpace(version) == "" {
		t.Fatalf("expected certificate version recorded in listing")
	}

	jobTicket := "mod-node"
	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      jobTicket,
		StepID:      "build",
		Priority:    "default",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	claim, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: workerID})
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}

	completed := make(chan struct{})
	go func() {
		defer close(completed)
		time.Sleep(150 * time.Millisecond)
		_, err := sched.CompleteJob(ctx, scheduler.CompleteRequest{
			JobID:  claim.Job.ID,
			NodeID: workerID,
			Ticket: job.Ticket,
			State:  scheduler.JobStateSucceeded,
		})
		if err != nil {
			t.Errorf("complete job: %v", err)
		}
	}()

	deleteStatus, _ := deleteJSONStatus(t, server.URL+"/v1/nodes", map[string]any{
		"cluster_id":            "cluster-alpha",
		"worker_id":             workerID,
		"confirm":               workerID,
		"drain_timeout_seconds": 5,
	})
	if deleteStatus != http.StatusNoContent {
		t.Fatalf("expected delete status 204, got %d", deleteStatus)
	}

	select {
	case <-completed:
	case <-time.After(2 * time.Second):
		t.Fatalf("job completion goroutine did not finish")
	}

	state, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("manager state: %v", err)
	}
	for _, id := range state.Nodes.Workers {
		if id == workerID {
			t.Fatalf("expected worker removed from CA inventory")
		}
	}
	if _, ok := state.WorkerCertificates[workerID]; ok {
		t.Fatalf("expected worker certificate removed from CA state")
	}

	reg, err := registry.NewWorkerRegistry(client, "cluster-alpha")
	if err != nil {
		t.Fatalf("new worker registry: %v", err)
	}
	if _, err := reg.Get(ctx, workerID); !errors.Is(err, registry.ErrWorkerNotFound) {
		t.Fatalf("expected registry ErrWorkerNotFound, got %v", err)
	}
}

func TestServerNodesRBACEnforced(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	server := httptest.NewServer(newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
		Authorizer: auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleWorker,
		}),
	}))
	defer server.Close()

	status, _ := postJSONStatus(t, server.URL+"/v1/nodes", map[string]any{
		"cluster_id": "cluster-rbac",
		"address":    "10.20.1.70",
	})
	if status != http.StatusForbidden {
		t.Fatalf("expected forbidden when role unauthorized, got %d", status)
	}
}

func TestServerNodeJoinAutoBootstrapsPKI(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	server := httptest.NewServer(newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	status, _ := postJSONStatus(t, server.URL+"/v1/nodes", map[string]any{
		"cluster_id": "cluster-auto",
		"address":    "10.20.1.60",
	})
	if status != http.StatusCreated {
		t.Fatalf("expected node join status 201, got %d", status)
	}

	manager, err := deploy.NewCARotationManager(client, "cluster-auto")
	if err != nil {
		t.Fatalf("NewCARotationManager: %v", err)
	}
	state, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("expected CA state after auto-bootstrap: %v", err)
	}
	if state.CurrentCA.Version == "" {
		t.Fatalf("expected CA version recorded after auto-bootstrap")
	}
}

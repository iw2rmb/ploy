package httpserver_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestServerSecurityCAStatus(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	mustBootstrapCluster(t, client, "cluster-alpha")

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

	status, payload := getJSONStatus(t, fmt.Sprintf("%s/v1/security/ca?cluster_id=%s", server.URL, url.QueryEscape("cluster-alpha")))
	if status != http.StatusOK {
		t.Fatalf("expected CA status 200, got %d", status)
	}
	current, _ := payload["current_ca"].(map[string]any)
	version, _ := current["version"].(string)
	if strings.TrimSpace(version) == "" {
		t.Fatalf("expected CA version in response")
	}
}

func TestControlPlaneCertificateBootstrap(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd: client,
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	payload := map[string]any{
		"cluster_id": "cluster-alpha",
		"node_id":    "control-primary",
		"address":    "203.0.113.10",
	}
	resp := postJSON(t, server.URL+"/v1/security/certificates/control-plane", payload)
	certificate, ok := resp["certificate"].(map[string]any)
	if !ok {
		t.Fatalf("certificate block missing from response: %v", resp)
	}
	if certNode, _ := certificate["node_id"].(string); certNode != "control-primary" {
		t.Fatalf("expected certificate node_id control-primary, got %q", certNode)
	}
	caBundle, _ := resp["ca_bundle"].(string)
	if strings.TrimSpace(caBundle) == "" {
		t.Fatalf("expected ca_bundle in response")
	}

	manager, err := deploy.NewCARotationManager(client, "cluster-alpha")
	if err != nil {
		t.Fatalf("new ca rotation manager: %v", err)
	}
	state, err := manager.State(context.Background())
	if err != nil {
		t.Fatalf("manager state: %v", err)
	}
	if _, ok := state.BeaconCertificates["control-primary"]; !ok {
		t.Fatalf("expected control-primary certificate recorded in etcd")
	}
	if got := strings.TrimSpace(state.CurrentCA.CertificatePEM); got != strings.TrimSpace(caBundle) {
		t.Fatalf("response ca_bundle mismatch with etcd state")
	}
}

func TestControlPlaneCertificateValidation(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd: client,
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	status, body := postJSONStatus(t, server.URL+"/v1/security/certificates/control-plane", map[string]any{
		"cluster_id": "cluster-beta",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected bad request for missing address, got %d", status)
	}
	if msg, _ := body["error"].(string); msg != "address is required" {
		t.Fatalf("unexpected error message: %v", body)
	}
}

package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

func TestHandleClusterCertStatus(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cluster_id") != "cluster-alpha" {
			t.Fatalf("expected cluster_id query, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"cluster_id":"cluster-alpha","current_ca":{"version":"20251025","issued_at":"2025-10-25T12:00:00Z","expires_at":"2026-10-25T12:00:00Z","serial_number":"abcd"},"workers":{"total":3},"trust_bundle_hash":"deadbeef"}`)
	}))
	defer server.Close()
	desc, err := config.SaveDescriptor(config.Descriptor{
		ClusterID:       "cluster-alpha",
		Address:         server.URL,
		SSHIdentityPath: "/home/ploy/.ssh/id_alpha",
	})
	if err != nil {
		t.Fatalf("save descriptor: %v", err)
	}
	origFactory := clusterHTTPClientFactory
	defer func() { clusterHTTPClientFactory = origFactory }()
	clusterHTTPClientFactory = func(config.Descriptor) (*http.Client, func(), error) {
		return server.Client(), func() {}, nil
	}
	buf := &bytes.Buffer{}
	if err := handleCluster([]string{"cert", "status", "--cluster-id", desc.ClusterID}, buf); err != nil {
		t.Fatalf("cluster cert status returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Version: 20251025") {
		t.Fatalf("expected version in output, got %q", output)
	}
	if !strings.Contains(output, "Workers: 3") {
		t.Fatalf("expected worker count in output, got %q", output)
	}
}

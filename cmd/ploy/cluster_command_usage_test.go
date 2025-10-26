package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

func TestHandleClusterRequiresSubcommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	buf := &bytes.Buffer{}
	err := handleCluster(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing cluster subcommand")
	}
	if buf.Len() == 0 {
		t.Fatalf("expected usage output when cluster subcommand missing")
	}
}

func TestHandleClusterListOutputsDescriptors(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	_, err := config.SaveDescriptor(config.Descriptor{
		ClusterID:       "alpha",
		Address:         "10.10.0.1",
		SSHIdentityPath: "/home/ploy/.ssh/id_alpha",
		Labels:          map[string]string{"env": "dev"},
	})
	if err != nil {
		t.Fatalf("save alpha: %v", err)
	}
	_, err = config.SaveDescriptor(config.Descriptor{
		ClusterID:       "beta",
		Address:         "10.10.0.2",
		SSHIdentityPath: "/home/ploy/.ssh/id_beta",
		Labels:          map[string]string{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("save beta: %v", err)
	}
	if err := config.SetDefault("beta"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	buf := &bytes.Buffer{}
	if err := handleCluster([]string{"list"}, buf); err != nil {
		t.Fatalf("handleCluster list: %v", err)
	}
	output := buf.String()
	if !containsAll(output,
		"alpha",
		"address=10.10.0.1",
		"identity=/home/ploy/.ssh/id_alpha",
		"labels=env=dev",
		"beta (default)",
		"address=10.10.0.2",
		"labels=env=prod") {
		t.Fatalf("unexpected cluster list output:\n%s", output)
	}
}

func TestHandleClusterConnectStoresDescriptor(t *testing.T) {
	t.Skip("cluster connect implementation pending")
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	const caBody = "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----\n"
	var configCalls, caCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/beacon/config":
			configCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer api-key" {
				t.Fatalf("expected Authorization header, got %q", got)
			}
			_, _ = w.Write([]byte(`{"control_plane_url":"https://api.example","version":"2025.10.21"}`))
		case "/v1/beacon/ca":
			caCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer api-key" {
				t.Fatalf("expected Authorization header for CA request, got %q", got)
			}
			_, _ = w.Write([]byte(caBody))
		default:
			t.Fatalf("unexpected beacon path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	buf := &bytes.Buffer{}
	args := []string{"connect", "--id", "lab", "--beacon-url", server.URL, "--api-key", "api-key", "--set-default"}
	if err := handleCluster(args, buf); err != nil {
		t.Fatalf("handleCluster connect: %v", err)
	}
	if configCalls != 1 || caCalls != 1 {
		t.Fatalf("expected 1 config and 1 ca call, got config=%d ca=%d", configCalls, caCalls)
	}
	desc, err := config.LoadDescriptor("lab")
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}
	if !desc.Default {
		t.Fatalf("expected descriptor marked default")
	}
	if desc.Address == "" {
		t.Fatalf("expected Address recorded, got empty")
	}
	if desc.SSHIdentityPath == "" {
		t.Fatalf("expected SSH identity recorded, got empty")
	}
	if buf.Len() == 0 {
		t.Fatalf("expected output summarizing cluster connection")
	}
}

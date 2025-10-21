package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/cmd/ploy/config"
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
		ID:              "alpha",
		BeaconURL:       "https://alpha-beacon",
		ControlPlaneURL: "https://alpha-control",
		Version:         "2025.09.01",
		LastRefreshed:   time.Date(2025, 9, 15, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("save alpha: %v", err)
	}
	_, err = config.SaveDescriptor(config.Descriptor{
		ID:              "beta",
		BeaconURL:       "https://beta-beacon",
		ControlPlaneURL: "https://beta-control",
		Version:         "2025.10.01",
		LastRefreshed:   time.Date(2025, 10, 1, 9, 30, 0, 0, time.UTC),
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
		"https://alpha-beacon",
		"https://alpha-control",
		"2025.09.01",
		"beta (default)",
		"https://beta-beacon",
		"https://beta-control") {
		t.Fatalf("unexpected cluster list output:\n%s", output)
	}
}

func containsAll(s string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(s, needle) {
			return false
		}
	}
	return true
}

func TestHandleClusterConnectStoresDescriptor(t *testing.T) {
	t.Skip("cluster connect implementation pending")
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	const caBody = "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----\n"
	var configCalls, caCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/beacon/config":
			configCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer api-key" {
				t.Fatalf("expected Authorization header, got %q", got)
			}
			_, _ = w.Write([]byte(`{"control_plane_url":"https://api.example","version":"2025.10.21"}`))
		case "/v2/beacon/ca":
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
	if desc.APIKey != "api-key" {
		t.Fatalf("expected API key persisted, got %q", desc.APIKey)
	}
	if desc.BeaconURL != server.URL {
		t.Fatalf("expected beacon url %q, got %q", server.URL, desc.BeaconURL)
	}
	if desc.ControlPlaneURL != "https://api.example" {
		t.Fatalf("expected control plane url, got %q", desc.ControlPlaneURL)
	}
	if desc.Version != "2025.10.21" {
		t.Fatalf("expected version persisted, got %q", desc.Version)
	}
	if desc.CABundlePath == "" {
		t.Fatalf("expected ca bundle path recorded")
	}
	data, err := os.ReadFile(desc.CABundlePath)
	if err != nil {
		t.Fatalf("read ca bundle: %v", err)
	}
	if string(data) != caBody {
		t.Fatalf("unexpected ca bundle contents: %q", string(data))
	}
	if desc.LastRefreshed.IsZero() {
		t.Fatalf("expected LastRefreshed to be set")
	}
	if buf.Len() == 0 {
		t.Fatalf("expected output summarizing cluster connection")
	}
}

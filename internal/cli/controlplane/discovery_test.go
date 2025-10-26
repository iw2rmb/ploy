package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/pkg/sshtransport"
)

func TestResolveTargetSelectsDefaultDescriptor(t *testing.T) {
	t.Setenv("PLOY_CONFIG_HOME", t.TempDir())
	stored, err := config.SaveDescriptor(config.Descriptor{ClusterID: "alpha", Address: "203.0.113.10"})
	if err != nil {
		t.Fatalf("SaveDescriptor: %v", err)
	}
	if _, err := ResolveTarget(context.Background(), Options{}); err != nil {
		t.Fatalf("ResolveTarget default: %v", err)
	}
	target, err := ResolveTarget(context.Background(), Options{})
	if err != nil {
		t.Fatalf("ResolveTarget default: %v", err)
	}
	if target.ClusterID != stored.ClusterID {
		t.Fatalf("unexpected cluster id: got %q want %q", target.ClusterID, stored.ClusterID)
	}
	expectBase, _ := url.Parse("https://203.0.113.10:8443")
	if target.BaseURL.String() != expectBase.String() {
		t.Fatalf("unexpected base url: got %q want %q", target.BaseURL, expectBase)
	}
	if target.Descriptor == nil {
		t.Fatal("expected descriptor attached to target")
	}
}

func TestResolveTargetHonorsClusterIDOverride(t *testing.T) {
	t.Setenv("PLOY_CONFIG_HOME", t.TempDir())
	if _, err := config.SaveDescriptor(config.Descriptor{ClusterID: "alpha", Address: "198.51.100.10"}); err != nil {
		t.Fatalf("SaveDescriptor alpha: %v", err)
	}
	saved, err := config.SaveDescriptor(config.Descriptor{ClusterID: "beta", Address: "198.51.100.20"})
	if err != nil {
		t.Fatalf("SaveDescriptor beta: %v", err)
	}
	target, err := ResolveTarget(context.Background(), Options{ClusterID: "beta"})
	if err != nil {
		t.Fatalf("ResolveTarget beta: %v", err)
	}
	if target.ClusterID != saved.ClusterID {
		t.Fatalf("unexpected target cluster: got %q want %q", target.ClusterID, saved.ClusterID)
	}
}

func TestResolveTargetFallsBackToEnvEndpoint(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configDir)
	if _, err := config.SaveDescriptor(config.Descriptor{ClusterID: "envtest", Address: "192.0.2.10"}); err != nil {
		t.Fatalf("SaveDescriptor: %v", err)
	}
	t.Setenv("PLOY_CONTROL_PLANE_URL", "https://control.example.test:9443")
	target, err := ResolveTarget(context.Background(), Options{})
	if err != nil {
		t.Fatalf("ResolveTarget env: %v", err)
	}
	if target.BaseURL.String() != "https://control.example.test:9443" {
		t.Fatalf("unexpected env base url: %q", target.BaseURL)
	}
}

func TestResolveTargetErrorsWhenMissingConfig(t *testing.T) {
	t.Setenv("PLOY_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	if _, err := ResolveTarget(context.Background(), Options{}); err == nil {
		t.Fatal("expected error when no descriptors and no env override")
	}
}

func TestResolveTargetSeedsDescriptorFromControlPlaneConfig(t *testing.T) {
	t.Setenv("PLOY_CONFIG_HOME", t.TempDir())
	t.Setenv("PLOY_CACHE_HOME", t.TempDir())
	origAttach := attachHTTPClient
	origSetNodes := setTunnelNodes
	origFallback := ensureFallbackTunnel
	attachHTTPClient = func(*http.Client) error { return nil }
	setTunnelNodes = func([]sshtransport.Node) error { return nil }
	ensureFallbackTunnel = func(*url.URL) error { return nil }
	t.Cleanup(func() {
		attachHTTPClient = origAttach
		setTunnelNodes = origSetNodes
		ensureFallbackTunnel = origFallback
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/config") {
			_, _ = w.Write([]byte(`{
			  "cluster_id": "alpha",
			  "config": {
			    "discovery": {
			      "default_descriptor": "alpha",
			      "descriptors": [
			        {"cluster_id":"alpha","address":"control.ssh","api_endpoint":"https://control.alpha:8443","ca_bundle":"CABUNDLE"}
			      ]
			    }
			  }
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("PLOY_CONTROL_PLANE_URL", server.URL)
	target, err := ResolveTarget(context.Background(), Options{ClusterID: "alpha"})
	if err != nil {
		t.Fatalf("ResolveTarget seed: %v", err)
	}
	if target.ClusterID != "alpha" {
		t.Fatalf("expected cluster alpha, got %s", target.ClusterID)
	}
	desc, err := config.LoadDescriptor("alpha")
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}
	if desc.Address != "control.ssh" {
		t.Fatalf("unexpected descriptor address %q", desc.Address)
	}
	if strings.TrimSpace(desc.CABundle) != "CABUNDLE" {
		t.Fatalf("expected CABUNDLE, got %q", desc.CABundle)
	}
}

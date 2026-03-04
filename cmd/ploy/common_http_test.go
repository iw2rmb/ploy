package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	cliconfig "github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestResolveControlPlaneHTTP_PlainWithHTTPDescriptor(t *testing.T) {
	// Descriptor with http scheme should yield a plain client.
	// Isolate config home to ensure tests never touch the real default.
	clienv.IsolateConfigHomeAllowDefault(t)

	if _, err := cliconfig.SaveDescriptor(cliconfig.Descriptor{ClusterID: cliconfig.ClusterID("c1"), Address: "http://127.0.0.1:9094"}); err != nil {
		t.Fatalf("SaveDescriptor: %v", err)
	}
	if err := cliconfig.SetDefault(cliconfig.ClusterID("c1")); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}

	u, client, err := resolveControlPlaneHTTP(context.TODO())
	if err != nil {
		t.Fatalf("resolveControlPlaneHTTP error: %v", err)
	}
	if got, want := u.Scheme, "http"; got != want {
		t.Fatalf("scheme=%s want %s", got, want)
	}
	if client == nil {
		t.Fatalf("expected client, got nil")
	}
	if client.Timeout <= 0 {
		t.Fatalf("expected default Timeout to be set, got %v", client.Timeout)
	}
}

func TestCloneForStreamDisablesTimeout(t *testing.T) {
	c := &http.Client{Timeout: 5 * time.Second}
	clone := cloneForStream(c)
	if clone.Timeout != 0 {
		t.Fatalf("expected stream clone Timeout=0, got %v", clone.Timeout)
	}
	if clone == c {
		t.Fatal("expected a distinct client clone instance")
	}
}

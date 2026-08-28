package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestNodeCommandsUseCanonicalPaths(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domainapi.Node{{ID: "abc123", Name: "worker"}})
		case "/api/v1/nodes/abc123/diagnostics":
			_ = json.NewEncoder(w).Encode([]domainapi.NodeDiagnostic{{NodeID: "abc123", Component: "node"}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	baseURL, _ := url.Parse(server.URL + "/api")

	nodes, err := (ListNodesCommand{Client: server.Client(), BaseURL: baseURL}).Run(context.Background())
	if err != nil || len(nodes) != 1 || nodes[0].Name != "worker" {
		t.Fatalf("list nodes = %+v, %v", nodes, err)
	}
	diagnostics, err := (ListNodeDiagnosticsCommand{
		Client: server.Client(), BaseURL: baseURL, NodeID: domaintypes.NodeID("abc123"),
	}).Run(context.Background())
	if err != nil || len(diagnostics) != 1 || diagnostics[0].Component != "node" {
		t.Fatalf("list diagnostics = %+v, %v", diagnostics, err)
	}
}

//go:build integration

package artifacts_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

func TestClusterClientLifecycle(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/add":
			if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/") {
				t.Fatalf("expected multipart content-type, got %s", ct)
			}
			_, _ = w.Write([]byte(`{"Name":"artifact.bin","Hash":"bafyartifact","Size":"12"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/ipfs/bafyartifact":
			_ = json.NewEncoder(w).Encode(map[string]string{"payload": "stub"})
		case r.Method == http.MethodGet && r.URL.Path == "/pins/bafyartifact":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"cid":         map[string]string{"/": "bafyartifact"},
				"name":        "artifact.bin",
				"pin_options": map[string]int{"replication_factor_min": 2, "replication_factor_max": 3},
				"status": map[string]any{
					"summary": "pinned",
					"peers": map[string]any{
						"12D3Kfoo": map[string]string{"status": "pinned"},
					},
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/pins/bafyartifact":
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := artifacts.NewClusterClient(artifacts.ClusterClientOptions{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}

	addResp, err := client.Add(context.Background(), artifacts.AddRequest{
		Name:    "artifact.bin",
		Payload: []byte("payload"),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if addResp.CID != "bafyartifact" {
		t.Fatalf("unexpected cid: %s", addResp.CID)
	}

	status, err := client.Status(context.Background(), "bafyartifact")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.ReplicationFactorMin != 2 || status.ReplicationFactorMax != 3 {
		t.Fatalf("unexpected replication factors: %+v", status)
	}
	if len(status.Peers) != 1 || status.Peers[0].Status != "pinned" || status.Peers[0].PeerID != "12D3Kfoo" {
		t.Fatalf("unexpected status peers: %+v", status.Peers)
	}

	if err := client.Unpin(context.Background(), "bafyartifact"); err != nil {
		t.Fatalf("Unpin: %v", err)
	}

	expected := []string{"POST /add", "GET /pins/bafyartifact", "DELETE /pins/bafyartifact"}
	for _, want := range expected {
		found := false
		for _, req := range requests {
			if req == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected request %s not observed: %v", want, requests)
		}
	}
}

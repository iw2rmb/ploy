package artifacts

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClusterClientAddPublishesArtifact(t *testing.T) {
	var captured *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/add" {
			t.Fatalf("expected /add path, got %s", r.URL.Path)
		}
        if got := r.URL.Query().Get("replication_factor_min"); got != "2" {
            t.Fatalf("expected replication_factor_min=2, got %s", got)
        }
        if got := r.URL.Query().Get("replication_factor_max"); got != "3" {
            t.Fatalf("expected replication_factor_max=3, got %s", got)
        }
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if !strings.Contains(string(body), "log payload") {
			t.Fatalf("expected multipart body to contain payload, got %q", string(body))
		}
		_, _ = w.Write([]byte(`{"Name":"artifact.log","Hash":"bafyartifact","Size":"11"}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClusterClient(ClusterClientOptions{
		BaseURL:              server.URL,
		AuthToken:            "secret-token",
		ReplicationFactorMin: 2,
		ReplicationFactorMax: 3,
	})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}

	result, err := client.Add(context.Background(), AddRequest{
		Name:    "artifact.log",
		Payload: []byte("log payload"),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if result.CID != "bafyartifact" {
		t.Fatalf("unexpected cid: %s", result.CID)
	}
	if result.Digest == "" || !strings.HasPrefix(result.Digest, "sha256:") {
		t.Fatalf("expected sha256 digest, got %q", result.Digest)
	}
	if result.Size != int64(len("log payload")) {
		t.Fatalf("expected size %d, got %d", len("log payload"), result.Size)
	}
	if captured == nil {
		t.Fatalf("expected request to be captured")
	}
	if auth := captured.Header.Get("Authorization"); auth != "Bearer secret-token" {
		t.Fatalf("expected bearer token auth, got %q", auth)
	}
	if contentType := captured.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Fatalf("expected multipart content-type, got %q", contentType)
	}
}

func TestClusterClientAddPropagatesServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "cluster failure", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	client, err := NewClusterClient(ClusterClientOptions{
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}

	_, err = client.Add(context.Background(), AddRequest{
		Name:    "artifact.tar",
		Payload: []byte("payload"),
	})
	if err == nil {
		t.Fatal("expected Add error when cluster returns 503")
	}
}

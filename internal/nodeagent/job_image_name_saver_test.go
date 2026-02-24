package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestJobImageNameSaver_SaveJobImageName_RequestShape(t *testing.T) {
	t.Parallel()

	var (
		gotPath  string
		gotImage string
		gotAuth  string
		gotNode  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotNode = r.Header.Get("PLOY_NODE_UUID")

		var payload struct {
			Image string `json:"image"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotImage = payload.Image

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	saver, err := NewJobImageNameSaver(cfg)
	if err != nil {
		t.Fatalf("NewJobImageNameSaver() error = %v", err)
	}

	jobID := types.JobID("test-job-id")
	if err := saver.SaveJobImageName(context.Background(), jobID, "docker.io/example/migs:latest"); err != nil {
		t.Fatalf("SaveJobImageName() error = %v", err)
	}

	if gotPath != "/v1/jobs/test-job-id/image" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/jobs/test-job-id/image")
	}
	if gotImage != "docker.io/example/migs:latest" {
		t.Fatalf("payload.image = %q, want %q", gotImage, "docker.io/example/migs:latest")
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
	if gotNode != testNodeID {
		t.Fatalf("PLOY_NODE_UUID = %q, want %q", gotNode, testNodeID)
	}
}

func TestJobImageNameSaver_SaveJobImageName_RetryOn5xx(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	saver, err := NewJobImageNameSaver(cfg)
	if err != nil {
		t.Fatalf("NewJobImageNameSaver() error = %v", err)
	}

	jobID := types.JobID("test-job-id")
	if err := saver.SaveJobImageName(context.Background(), jobID, "docker.io/example/migs:latest"); err != nil {
		t.Fatalf("SaveJobImageName() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want %d", attempts, 3)
	}
}

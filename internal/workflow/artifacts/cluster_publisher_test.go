package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

type fakeAddClient struct {
	requests []AddRequest
	response AddResponse
	err      error
}

func (f *fakeAddClient) Add(ctx context.Context, req AddRequest) (AddResponse, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return AddResponse{}, f.err
	}
	return f.response, nil
}

func TestClusterPublisherPublishReadsFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.log")
	if err := os.WriteFile(path, []byte("artifact-data"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	client := &fakeAddClient{
		response: AddResponse{
			CID:    "bafyartifact",
			Digest: "sha256:deadbeef",
			Size:   13,
			Name:   "artifact.log",
		},
	}
	publisher, err := NewClusterPublisher(ClusterPublisherOptions{
		Client:      client,
		NameBuilder: func(kind step.ArtifactKind) string { return "artifact.log" },
	})
	if err != nil {
		t.Fatalf("NewClusterPublisher: %v", err)
	}

	result, err := publisher.Publish(context.Background(), step.ArtifactRequest{
		Kind: step.ArtifactKindLogs,
		Path: path,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.CID != "bafyartifact" {
		t.Fatalf("unexpected cid: %s", result.CID)
	}
	if result.Kind != step.ArtifactKindLogs {
		t.Fatalf("unexpected kind: %s", result.Kind)
	}
	if result.Digest != "sha256:deadbeef" {
		t.Fatalf("unexpected digest: %s", result.Digest)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected single Add invocation, got %d", len(client.requests))
	}
	if client.requests[0].Name != "artifact.log" {
		t.Fatalf("expected request name artifact.log, got %q", client.requests[0].Name)
	}
	payload := string(client.requests[0].Payload)
	if payload != "artifact-data" {
		t.Fatalf("unexpected payload: %q", payload)
	}
}

func TestClusterPublisherPublishUsesBufferFallback(t *testing.T) {
	client := &fakeAddClient{
		response: AddResponse{
			CID:    "bafybuffer",
			Digest: "sha256:cafebabe",
			Name:   "diff.tar",
			Size:   12,
		},
	}
	publisher, err := NewClusterPublisher(ClusterPublisherOptions{
		Client: client,
		NameBuilder: func(kind step.ArtifactKind) string {
			if kind == step.ArtifactKindDiff {
				return "diff.tar"
			}
			return "artifact.bin"
		},
	})
	if err != nil {
		t.Fatalf("NewClusterPublisher: %v", err)
	}

	result, err := publisher.Publish(context.Background(), step.ArtifactRequest{
		Kind:   step.ArtifactKindDiff,
		Buffer: []byte("diff-payload"),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.CID != "bafybuffer" {
		t.Fatalf("unexpected cid: %s", result.CID)
	}
	if result.Digest != "sha256:cafebabe" {
		t.Fatalf("unexpected digest: %s", result.Digest)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected single Add invocation")
	}
	if client.requests[0].Name != "diff.tar" {
		t.Fatalf("unexpected request name: %s", client.requests[0].Name)
	}
	if string(client.requests[0].Payload) != "diff-payload" {
		t.Fatalf("unexpected payload: %q", string(client.requests[0].Payload))
	}
}

func TestClusterPublisherPublishValidatesPayload(t *testing.T) {
	client := &fakeAddClient{}
	publisher, err := NewClusterPublisher(ClusterPublisherOptions{
		Client: client,
		NameBuilder: func(kind step.ArtifactKind) string {
			return "artifact.bin"
		},
	})
	if err != nil {
		t.Fatalf("NewClusterPublisher: %v", err)
	}

	_, err = publisher.Publish(context.Background(), step.ArtifactRequest{
		Kind: step.ArtifactKindLogs,
	})
	if err == nil {
		t.Fatal("expected error when request lacks path and buffer")
	}
	if !strings.Contains(err.Error(), "artifact payload required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

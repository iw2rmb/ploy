package artifacts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
	if result.Size != 13 {
		t.Fatalf("unexpected size: %d", result.Size)
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
	if result.Size != 12 {
		t.Fatalf("unexpected size: %d", result.Size)
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

func TestClusterPublisherDeduplicatesByDigest(t *testing.T) {
	client := &fakeAddClient{
		response: AddResponse{
			CID:    "bafy-dedupe",
			Digest: "sha256:dedupe",
			Name:   "logs-123.txt",
			Size:   20,
		},
	}
	publisher, err := NewClusterPublisher(ClusterPublisherOptions{
		Client: client,
		NameBuilder: func(kind step.ArtifactKind) string {
			return "logs-123.txt"
		},
	})
	if err != nil {
		t.Fatalf("NewClusterPublisher: %v", err)
	}

	payload := []byte("duplicate log payload")
	first, err := publisher.Publish(context.Background(), step.ArtifactRequest{
		Kind:   step.ArtifactKindLogs,
		Buffer: payload,
	})
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	second, err := publisher.Publish(context.Background(), step.ArtifactRequest{
		Kind:   step.ArtifactKindLogs,
		Buffer: payload,
	})
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected single Add call, got %d", len(client.requests))
	}
	if second.CID != first.CID {
		t.Fatalf("expected deduplicated CID, got %s vs %s", second.CID, first.CID)
	}
}

func TestClusterPublisherRetriesOnFailure(t *testing.T) {
	client := &sequenceAddClient{
		results: []AddResponse{
			{
				CID:    "bafy-retry",
				Digest: "sha256:retry",
				Name:   "logs.txt",
				Size:   16,
			},
		},
		errors: []error{
			fmt.Errorf("transient failure"),
			nil,
		},
	}
	publisher, err := NewClusterPublisher(ClusterPublisherOptions{
		Client:     client,
		MaxRetries: 1,
		RetryDelay: time.Millisecond,
		NameBuilder: func(step.ArtifactKind) string {
			return "logs.txt"
		},
	})
	if err != nil {
		t.Fatalf("NewClusterPublisher: %v", err)
	}
	result, err := publisher.Publish(context.Background(), step.ArtifactRequest{
		Kind:   step.ArtifactKindLogs,
		Buffer: []byte("retry payload"),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if client.failures != 1 {
		t.Fatalf("expected single retry, got %d failures", client.failures)
	}
	if result.CID != "bafy-retry" {
		t.Fatalf("unexpected cid: %s", result.CID)
	}
}

func TestClusterPublisherEmitsMetrics(t *testing.T) {
	recorder := &fakeBundleRecorder{}
	client := &sequenceAddClient{
		results: []AddResponse{
			{
				CID:    "bafy-metric",
				Digest: "sha256:metric",
				Name:   "logs.txt",
				Size:   4,
			},
		},
		errors: []error{
			fmt.Errorf("pin failed"),
			nil,
		},
	}
	publisher, err := NewClusterPublisher(ClusterPublisherOptions{
		Client:     client,
		Recorder:   recorder,
		MaxRetries: 1,
		RetryDelay: time.Millisecond,
		NameBuilder: func(step.ArtifactKind) string {
			return "logs.txt"
		},
	})
	if err != nil {
		t.Fatalf("NewClusterPublisher: %v", err)
	}

	if _, err := publisher.Publish(context.Background(), step.ArtifactRequest{
		Kind:   step.ArtifactKindLogs,
		Buffer: []byte("data"),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if recorder.failures != 1 {
		t.Fatalf("expected one failure recorded, got %d", recorder.failures)
	}
	if recorder.retries != 1 {
		t.Fatalf("expected one retry recorded, got %d", recorder.retries)
	}
	if recorder.successes != 1 {
		t.Fatalf("expected one success recorded, got %d", recorder.successes)
	}
	if recorder.lastKind != string(step.ArtifactKindLogs) {
		t.Fatalf("unexpected kind recorded: %s", recorder.lastKind)
	}
}

type fakeBundleRecorder struct {
	failures  int
	retries   int
	successes int
	lastKind  string
}

func (f *fakeBundleRecorder) PinSuccess(kind string, _ time.Duration) {
	f.successes++
	f.lastKind = kind
}

func (f *fakeBundleRecorder) PinFailure(kind string, _ error) {
	f.failures++
	f.lastKind = kind
}

func (f *fakeBundleRecorder) PinRetry(kind string) {
	f.retries++
	f.lastKind = kind
}

type sequenceAddClient struct {
	mu       sync.Mutex
	requests []AddRequest
	results  []AddResponse
	errors   []error
	failures int
}

func (c *sequenceAddClient) Add(ctx context.Context, req AddRequest) (AddResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	if len(c.errors) > 0 {
		err := c.errors[0]
		c.errors = c.errors[1:]
		if err != nil {
			c.failures++
			return AddResponse{}, err
		}
	}
	if len(c.results) == 0 {
		return AddResponse{}, nil
	}
	resp := c.results[0]
	c.results = c.results[1:]
	return resp, nil
}

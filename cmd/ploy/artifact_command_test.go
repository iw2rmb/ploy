package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/artifacts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

type fakeArtifactClient struct {
	addReqs    []artifacts.AddRequest
	addResp    artifacts.AddResponse
	addErr     error
	fetchCID   string
	fetchResp  artifacts.FetchResult
	fetchErr   error
	unpinCID   string
	unpinErr   error
	statusCID  string
	statusResp artifacts.StatusResult
	statusErr  error
}

func (c *fakeArtifactClient) Add(ctx context.Context, req artifacts.AddRequest) (artifacts.AddResponse, error) {
	c.addReqs = append(c.addReqs, req)
	if c.addErr != nil {
		return artifacts.AddResponse{}, c.addErr
	}
	return c.addResp, nil
}

func (c *fakeArtifactClient) Fetch(ctx context.Context, cid string) (artifacts.FetchResult, error) {
	c.fetchCID = cid
	if c.fetchErr != nil {
		return artifacts.FetchResult{}, c.fetchErr
	}
	return c.fetchResp, nil
}

func (c *fakeArtifactClient) Unpin(ctx context.Context, cid string) error {
	c.unpinCID = cid
	return c.unpinErr
}

func (c *fakeArtifactClient) Status(ctx context.Context, cid string) (artifacts.StatusResult, error) {
	c.statusCID = cid
	if c.statusErr != nil {
		return artifacts.StatusResult{}, c.statusErr
	}
	return c.statusResp, nil
}

func TestHandleArtifactRoutesSubcommands(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleArtifact(nil, buf)
	if err == nil {
		t.Fatal("expected error when no subcommand provided")
	}
	if !strings.Contains(buf.String(), "Usage: ploy artifact") {
		t.Fatalf("expected usage in output, got %q", buf.String())
	}

	err = handleArtifact([]string{"unknown"}, buf)
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestHandleArtifactPushUploadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.log")
	if err := os.WriteFile(path, []byte("payload-data"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	client := &fakeArtifactClient{
		addResp: artifacts.AddResponse{
			CID:    "bafyartifact",
			Digest: "sha256:deadbeef",
			Size:   int64(len("payload-data")),
			Name:   "artifact.log",
		},
	}
	prevFactory := artifactClientFactory
	t.Cleanup(func() { artifactClientFactory = prevFactory })
	artifactClientFactory = func() (artifactService, error) {
		return client, nil
	}

	buf := &bytes.Buffer{}
	err := handleArtifactPush([]string{"--name", "artifact.log", path}, buf)
	if err != nil {
		t.Fatalf("handleArtifactPush: %v", err)
	}
	if len(client.addReqs) != 1 {
		t.Fatalf("expected single Add request, got %d", len(client.addReqs))
	}
	req := client.addReqs[0]
	if req.Name != "artifact.log" {
		t.Fatalf("expected request name artifact.log, got %s", req.Name)
	}
	if string(req.Payload) != "payload-data" {
		t.Fatalf("unexpected request payload: %q", string(req.Payload))
	}
	if req.Kind != step.ArtifactKindLogs {
		t.Fatalf("expected default kind logs, got %s", req.Kind)
	}
	if !strings.Contains(buf.String(), "CID: bafyartifact") {
		t.Fatalf("expected output to include CID, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "Digest: sha256:deadbeef") {
		t.Fatalf("expected digest output, got %q", buf.String())
	}
}

func TestHandleArtifactPushPropagatesClientError(t *testing.T) {
	prevFactory := artifactClientFactory
	t.Cleanup(func() { artifactClientFactory = prevFactory })
	artifactClientFactory = func() (artifactService, error) {
		return nil, errors.New("client init failed")
	}

	buf := &bytes.Buffer{}
	err := handleArtifactPush([]string{"/tmp/artifact.bin"}, buf)
	if err == nil {
		t.Fatal("expected error when factory fails")
	}
}

func TestHandleArtifactPullWritesOutput(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "artifact.out")
	client := &fakeArtifactClient{
		fetchResp: artifacts.FetchResult{
			CID:    "bafyartifact",
			Digest: "sha256:deadbeef",
			Data:   []byte("artifact-bytes"),
		},
	}
	prevFactory := artifactClientFactory
	t.Cleanup(func() { artifactClientFactory = prevFactory })
	artifactClientFactory = func() (artifactService, error) {
		return client, nil
	}

	buf := &bytes.Buffer{}
	err := handleArtifactPull([]string{"--output", outputPath, "bafyartifact"}, buf)
	if err != nil {
		t.Fatalf("handleArtifactPull: %v", err)
	}
	if client.fetchCID != "bafyartifact" {
		t.Fatalf("expected fetch for cid, got %s", client.fetchCID)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(content) != "artifact-bytes" {
		t.Fatalf("unexpected output content: %q", string(content))
	}
	if !strings.Contains(buf.String(), "Wrote artifact to") {
		t.Fatalf("expected completion message, got %q", buf.String())
	}
}

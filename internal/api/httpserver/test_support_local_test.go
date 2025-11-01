package httpserver_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/api/config"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

// loadConfig writes the given YAML to a temp file and loads it via the config loader.
func loadConfig(t *testing.T, yaml string) config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

// stubStatus implements StatusProvider for node endpoints.
type stubStatus struct {
	snapshot map[string]any
	err      error
}

func (s *stubStatus) Snapshot(_ context.Context) (map[string]any, error) {
	return s.snapshot, s.err
}

// storedPayload holds in-memory artifact bytes for Fetch.
type storedPayload struct {
	data   []byte
	digest string
	media  string
}

// stubArtifactPublisher implements the minimal artifact publisher interface used by tests.
type stubArtifactPublisher struct {
	response workflowartifacts.AddResponse
	payloads map[string]storedPayload
}

func (s *stubArtifactPublisher) Add(_ context.Context, req workflowartifacts.AddRequest) (workflowartifacts.AddResponse, error) {
	if s.response.CID != "" {
		return s.response, nil
	}
	// Fabricate a response based on request payload.
	dg := workflowartifacts.SHA256Bytes(req.Payload)
	return workflowartifacts.AddResponse{
		CID:    "bafy" + dg[:6],
		Name:   req.Name,
		Size:   int64(len(req.Payload)),
		Digest: "sha256:" + dg,
	}, nil
}

func (s *stubArtifactPublisher) Fetch(_ context.Context, cid string) (workflowartifacts.FetchResult, error) {
	if p, ok := s.payloads[cid]; ok {
		media := p.media
		if media == "" {
			media = "application/octet-stream"
		}
		return workflowartifacts.FetchResult{CID: cid, Digest: p.digest, Data: p.data, Size: int64(len(p.data)), MediaType: media}, nil
	}
	return workflowartifacts.FetchResult{}, os.ErrNotExist
}

package httpserver_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

type stubArtifactPublisher struct {
	response workflowartifacts.AddResponse
	err      error
	lastReq  workflowartifacts.AddRequest
	payloads map[string]storedPayload
}

func (s *stubArtifactPublisher) Add(ctx context.Context, req workflowartifacts.AddRequest) (workflowartifacts.AddResponse, error) {
	s.lastReq = req
	if s == nil {
		return workflowartifacts.AddResponse{}, errors.New("stub publisher missing")
	}
	if s.err != nil {
		return workflowartifacts.AddResponse{}, s.err
	}
	resp := s.response
	if resp.CID == "" {
		resp.CID = "bafy-stub"
	}
	if resp.Digest == "" {
		resp.Digest = "sha256:stub"
	}
	if resp.Size == 0 {
		resp.Size = int64(len(req.Payload))
	}
	if resp.Name == "" {
		resp.Name = req.Name
	}
	if s.payloads == nil {
		s.payloads = make(map[string]storedPayload)
	}
	s.payloads[resp.CID] = storedPayload{data: append([]byte(nil), req.Payload...), digest: resp.Digest}
	return resp, nil
}

func (s *stubArtifactPublisher) Fetch(ctx context.Context, cid string) (workflowartifacts.FetchResult, error) {
	if s == nil {
		return workflowartifacts.FetchResult{}, errors.New("stub publisher missing")
	}
	payload, ok := s.payloads[strings.TrimSpace(cid)]
	if !ok {
		return workflowartifacts.FetchResult{}, fmt.Errorf("stub fetch: cid %s not found", cid)
	}
	data := append([]byte(nil), payload.data...)
	return workflowartifacts.FetchResult{
		CID:    cid,
		Data:   data,
		Size:   int64(len(data)),
		Digest: payload.digest,
	}, nil
}

type storedPayload struct {
	data   []byte
	digest string
}

type registryHTTPFixture struct {
	server    *httptest.Server
	store     *registry.Store
	publisher *stubArtifactPublisher
	baseDir   string
}

func newRegistryHTTPFixture(t *testing.T) registryHTTPFixture {
	t.Helper()
	etcd, client := startTestEtcd(t)
	store, err := registry.NewStore(client, registry.StoreOptions{})
	if err != nil {
		t.Fatalf("new registry store: %v", err)
	}
	publisher := &stubArtifactPublisher{}
	baseDir := t.TempDir()
	transfersMgr := transfers.NewManager(transfers.Options{BaseDir: baseDir})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd:              client,
		Transfers:         transfersMgr,
		ArtifactPublisher: publisher,
		RegistryStore:     store,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
		client.Close()
		etcd.Close()
	})
	return registryHTTPFixture{
		server:    server,
		store:     store,
		publisher: publisher,
		baseDir:   baseDir,
	}
}

func (f registryHTTPFixture) localPath(remote string) string {
	clean := filepath.Clean(strings.TrimSpace(remote))
	clean = strings.TrimPrefix(clean, "/")
	return filepath.Join(f.baseDir, clean)
}

func uploadRegistryBlob(t *testing.T, fixture registryHTTPFixture, repo string, payload []byte, mediaType string) string {
	t.Helper()
	uploadURL := fmt.Sprintf("%s/v1/registry/%s/blobs/uploads", fixture.server.URL, repo)
	status, startResp := postJSONStatus(t, uploadURL, map[string]any{
		"media_type": mediaType,
		"size":       len(payload),
	})
	if status != http.StatusAccepted {
		t.Fatalf("expected 202 for upload start, got %d", status)
	}
	slotID := requireString(t, startResp["upload_id"])
	remotePath := requireString(t, startResp["remote_path"])
	localPath := fixture.localPath(remotePath)
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("prepare slot dir: %v", err)
	}
	if err := os.WriteFile(localPath, payload, 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	patchURL := fmt.Sprintf("%s/v1/registry/%s/blobs/uploads/%s", fixture.server.URL, repo, slotID)
	if patchStatus, _ := sendJSONStatus(t, http.MethodPatch, patchURL, map[string]any{"size": len(payload)}); patchStatus != http.StatusAccepted {
		t.Fatalf("expected 202 for upload patch, got %d", patchStatus)
	}
	digest := sha256Digest(payload)
	values := url.Values{}
	values.Set("digest", digest)
	finalizeURL := fmt.Sprintf("%s?%s", patchURL, values.Encode())
	status, _ = sendJSONStatus(t, http.MethodPut, finalizeURL, map[string]any{
		"media_type": mediaType,
		"size":       len(payload),
	})
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for upload commit, got %d", status)
	}
	return digest
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func requireString(t *testing.T, value any) string {
	t.Helper()
	str, ok := value.(string)
	if !ok {
		t.Fatalf("expected string value, got %T", value)
	}
	return str
}

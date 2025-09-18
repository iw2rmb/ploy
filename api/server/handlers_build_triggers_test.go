package server

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	istorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

func TestHandleTriggerAppBuild_AsyncAccepted(t *testing.T) {
	// Intentionally do NOT set up storage; async acceptance should not require it

	// Redirect uploads path to a temp dir to avoid permission issues and allow 202 path
	dir := t.TempDir()
	old := uploadsBaseDir
	uploadsBaseDir = dir
	t.Cleanup(func() { uploadsBaseDir = old })

	s := createMockServer()
	s.app = fiber.New()
	s.app.Post("/v1/apps/:app/builds", s.handleTriggerAppBuild)

	req := httptest.NewRequest(http.MethodPost, "/v1/apps/demo/builds?async=true", bytes.NewBufferString("payload"))
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := s.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	if depID := resp.Header.Get("X-Deployment-ID"); depID == "" {
		t.Fatalf("expected X-Deployment-ID header on 202 acceptance")
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["id"].(string); !ok {
		t.Fatalf("expected id in response: %#v", body)
	}
}

func TestHandleTriggerAppBuild_NonDockerLaneRejected(t *testing.T) {
	// In-memory storage
	orig := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) { return memory.NewMemoryStorage(0), nil }
	t.Cleanup(func() { resolveStorageFromConfigService = orig })

	// uploads dir for async artifacts (not used in this sync path, but keep consistent)
	dir := t.TempDir()
	old := uploadsBaseDir
	uploadsBaseDir = dir
	t.Cleanup(func() { uploadsBaseDir = old })

	s := createMockServer()
	s.app = fiber.New()
	s.app.Post("/v1/apps/:app/builds", s.handleTriggerAppBuild)

	// Build a minimal tar without Dockerfile
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "README.txt", Mode: 0600, Size: int64(len("hi"))})
	_, _ = tw.Write([]byte("hi"))
	_ = tw.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/apps/demo/builds?lane=E", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := s.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error object: %#v", body)
	}
	if code := errObj["code"]; code != "lane_disabled" {
		t.Fatalf("unexpected error code %#v", errObj)
	}
	if supported := body["supported_lane"]; supported != "D" {
		t.Fatalf("expected supported_lane D, got %#v", supported)
	}
	if _, ok := body["builder"]; ok {
		t.Fatalf("builder object should not be present: %#v", body)
	}
}

func TestHandleTriggerBuild_InvalidAppName(t *testing.T) {
	// Resolve storage to in-memory to reach validation
	orig := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) { return memory.NewMemoryStorage(0), nil }
	t.Cleanup(func() { resolveStorageFromConfigService = orig })

	s := createMockServer()
	s.app = fiber.New()
	s.app.Post("/v1/builds/:app", s.handleTriggerBuild)

	req := httptest.NewRequest(http.MethodPost, "/v1/builds/INVALID!", bytes.NewBufferString("tar"))
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := s.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

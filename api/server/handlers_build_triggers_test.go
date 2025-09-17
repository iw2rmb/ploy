package server

import (
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
	// Resolve storage to in-memory to pass early checks
	orig := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) { return memory.NewMemoryStorage(0), nil }
	t.Cleanup(func() { resolveStorageFromConfigService = orig })

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
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["id"].(string); !ok {
		t.Fatalf("expected id in response: %#v", body)
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

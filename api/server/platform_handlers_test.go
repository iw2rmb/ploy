package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	istorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

func TestHandlePlatformDeploy_StorageResolutionFailure(t *testing.T) {
	orig := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { resolveStorageFromConfigService = orig })

	s := createMockServer()
	app := fiber.New()
	app.Post("/v1/platform/:service/deploy", s.handlePlatformDeploy)

	req := httptest.NewRequest(http.MethodPost, "/v1/platform/api/deploy", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestHandlePlatformStatus_NotFound(t *testing.T) {
	t.Skip("Infra-heavy: covered by e2e on VPS")
	// Storage is resolved but platform handler uses health monitor which will return not found locally
	orig := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) { return memory.NewMemoryStorage(0), nil }
	t.Cleanup(func() { resolveStorageFromConfigService = orig })

	s := createMockServer()
	app := fiber.New()
	app.Get("/v1/platform/:service/status", s.handlePlatformStatus)
	_ = app
}

func TestHandlePlatformLogs_ErrorPath(t *testing.T) {
	t.Skip("Infra-heavy: covered by e2e on VPS")
	s := createMockServer()
	app := fiber.New()
	app.Get("/v1/platform/:service/logs", s.handlePlatformLogs)
	_ = app
}

func TestHandlePlatformRollbackAndRemove(t *testing.T) {
	s := createMockServer()
	app := fiber.New()
	app.Post("/v1/platform/:service/rollback", s.handlePlatformRollback)
	app.Delete("/v1/platform/:service", s.handlePlatformRemove)

	// Rollback requires version query
	req := httptest.NewRequest(http.MethodPost, "/v1/platform/api/rollback?version=v1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Remove route returns success JSON
	resp2, err := app.Test(httptest.NewRequest(http.MethodDelete, "/v1/platform/api", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
}

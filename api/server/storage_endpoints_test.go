package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	istorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

func TestHandleStorageHealth_SuccessAndFailure(t *testing.T) {
	s := createMockServer()
	app := fiber.New()
	app.Get("/v1/storage/health", s.handleStorageHealth)

	// Success: resolver returns in-memory storage
	orig := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) { return memory.NewMemoryStorage(0), nil }
	t.Cleanup(func() { resolveStorageFromConfigService = orig })

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/storage/health", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Failure: resolver returns error
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) { return nil, assertErr }
	resp2, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/storage/health", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp2.StatusCode)
	}
}

var assertErr = struct{ error }{errorString("fail")}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestHandleStorageMetrics_Success(t *testing.T) {
	s := createMockServer()
	app := fiber.New()
	app.Get("/v1/storage/metrics", s.handleStorageMetrics)

	// Return in-memory storage
	orig := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) { return memory.NewMemoryStorage(0), nil }
	t.Cleanup(func() { resolveStorageFromConfigService = orig })

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/storage/metrics", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestStorageConfigEndpoints(t *testing.T) {
	s := createMockServer()
	app := fiber.New()
	app.Get("/v1/storage/config", s.handleGetStorageConfig)
	app.Post("/v1/storage/config/reload", s.handleReloadStorageConfig)
	app.Post("/v1/storage/config/validate", s.handleValidateStorageConfig)

	// Nil config service → errors
	resp1, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/storage/config", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp1.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp1.StatusCode)
	}

	resp2, err := app.Test(httptest.NewRequest(http.MethodPost, "/v1/storage/config/reload", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp2.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp2.StatusCode)
	}

	resp3, err := app.Test(httptest.NewRequest(http.MethodPost, "/v1/storage/config/validate", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp3.StatusCode)
	}

	// Provide a minimal config service
	svc, err := cfgsvc.New(cfgsvc.WithDefaults(&cfgsvc.Config{Storage: cfgsvc.StorageConfig{Provider: "memory", Endpoint: "local", Bucket: "artifacts"}}))
	if err != nil {
		t.Fatalf("config service: %v", err)
	}
	s.configService = svc

	resp4, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/storage/config", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp4.StatusCode)
	}

	resp5, err := app.Test(httptest.NewRequest(http.MethodPost, "/v1/storage/config/reload", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp5.StatusCode)
	}

	resp6, err := app.Test(httptest.NewRequest(http.MethodPost, "/v1/storage/config/validate", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp6.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp6.StatusCode)
	}
}

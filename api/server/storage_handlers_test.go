package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	istorage "github.com/iw2rmb/ploy/internal/storage"
)

// override resolveStorageFromConfigService via a test helper variable if available
// Otherwise, inject through Server.getStorageClient by replacing method on receiver via embedding not feasible.

func TestHandleStorageHealth_Healthy(t *testing.T) {
	s := &Server{dependencies: &ServiceDependencies{}}
	// Monkey patch resolveStorageFromConfigService via package-level var by wrapping helper
	old := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) {
		return &testStorage{healthy: true, metrics: istorage.NewStorageMetrics()}, nil
	}
	defer func() { resolveStorageFromConfigService = old }()

	app := fiber.New()
	app.Get("/v1/storage/health", s.handleStorageHealth)
	req := httptest.NewRequest("GET", "/v1/storage/health", nil)
	resp1, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp1 != nil && resp1.Body != nil {
			_ = resp1.Body.Close()
		}
	})
	if resp1.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp1.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp1.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "healthy" {
		t.Fatalf("expected healthy, got %#v", body["status"])
	}
}

func TestHandleStorageHealth_Unhealthy(t *testing.T) {
	s := &Server{dependencies: &ServiceDependencies{}}
	old := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) {
		return &testStorage{healthy: false, metrics: istorage.NewStorageMetrics()}, nil
	}
	defer func() { resolveStorageFromConfigService = old }()

	app := fiber.New()
	app.Get("/v1/storage/health", s.handleStorageHealth)
	req := httptest.NewRequest("GET", "/v1/storage/health", nil)
	resp2, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp2 != nil && resp2.Body != nil {
			_ = resp2.Body.Close()
		}
	})
	if resp2.StatusCode != 503 {
		t.Fatalf("expected 503, got %d", resp2.StatusCode)
	}
}

func TestHandleStorageMetrics(t *testing.T) {
	s := &Server{dependencies: &ServiceDependencies{}}
	metrics := istorage.NewStorageMetrics()
	metrics.RecordUpload(true, 0, 123, istorage.ErrorType(""))
	old := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) {
		return &testStorage{healthy: true, metrics: metrics}, nil
	}
	defer func() { resolveStorageFromConfigService = old }()

	app := fiber.New()
	app.Get("/v1/storage/metrics", s.handleStorageMetrics)
	req := httptest.NewRequest("GET", "/v1/storage/metrics", nil)
	resp3, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp3 != nil && resp3.Body != nil {
			_ = resp3.Body.Close()
		}
	})
	if resp3.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	var snap istorage.StorageMetrics
	if err := json.NewDecoder(resp3.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.SuccessfulUploads != 1 {
		t.Fatalf("expected SuccessfulUploads=1, got %d", snap.SuccessfulUploads)
	}
}

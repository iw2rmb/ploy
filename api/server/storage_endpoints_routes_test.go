package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/health"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
)

func TestStorageEndpoints_WithFullServerRoutes(t *testing.T) {
	// Minimal config service that resolves to in-memory storage
	svc, err := cfgsvc.New(cfgsvc.WithDefaults(&cfgsvc.Config{Storage: cfgsvc.StorageConfig{Provider: "memory", Endpoint: "local", Bucket: "artifacts"}}))
	if err != nil {
		t.Fatalf("config service: %v", err)
	}

	s := &Server{
		app:           fiber.New(),
		config:        &ControllerConfig{Port: "18081"},
		dependencies:  &ServiceDependencies{HealthChecker: health.NewHealthChecker("", "")},
		configService: svc,
	}

	s.setupRoutes()

	// /v1/storage/health
	resp1, err := s.app.Test(httptest.NewRequest(http.MethodGet, "/v1/storage/health", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp1.Body.Close() }()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp1.StatusCode)
	}

	// /v1/storage/metrics
	resp2, err := s.app.Test(httptest.NewRequest(http.MethodGet, "/v1/storage/metrics", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	// /v1/storage/config
	resp3, err := s.app.Test(httptest.NewRequest(http.MethodGet, "/v1/storage/config", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp3.Body.Close() }()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}

	// /v1/storage/config/reload
	resp4, err := s.app.Test(httptest.NewRequest(http.MethodPost, "/v1/storage/config/reload", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp4.Body.Close() }()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp4.StatusCode)
	}

	// /v1/storage/config/validate
	resp5, err := s.app.Test(httptest.NewRequest(http.MethodPost, "/v1/storage/config/validate", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp5.Body.Close() }()
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp5.StatusCode)
	}
}

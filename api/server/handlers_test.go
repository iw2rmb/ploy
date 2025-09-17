package server

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
)

func newMemoryConfigService() *cfgsvc.Service {
	svc, _ := cfgsvc.New(
		cfgsvc.WithDefaults(&cfgsvc.Config{
			App:     cfgsvc.AppConfig{Name: "test", Version: "test"},
			Storage: cfgsvc.StorageConfig{Provider: "memory"},
		}),
	)
	return svc
}

func TestStorageHandlersWithConfigService(t *testing.T) {
	svc := newMemoryConfigService()
	s := &Server{configService: svc, dependencies: &ServiceDependencies{}}
	app := fiber.New()
	app.Get("/health", s.handleStorageHealth)
	app.Get("/metrics", s.handleStorageMetrics)
	app.Get("/cfg", s.handleGetStorageConfig)
	app.Post("/cfg/reload", s.handleReloadStorageConfig)
	app.Post("/cfg/validate", s.handleValidateStorageConfig)

	resp := mustResponse(t)(app.Test(httptest.NewRequest("GET", "/health", nil)))
	if resp.StatusCode != 200 {
		t.Fatalf("storage health expected 200, got %d", resp.StatusCode)
	}

	resp = mustResponse(t)(app.Test(httptest.NewRequest("GET", "/metrics", nil)))
	if resp.StatusCode != 200 {
		t.Fatalf("storage metrics expected 200, got %d", resp.StatusCode)
	}

	resp = mustResponse(t)(app.Test(httptest.NewRequest("GET", "/cfg", nil)))
	if resp.StatusCode != 200 {
		t.Fatalf("get storage config expected 200, got %d", resp.StatusCode)
	}

	resp = mustResponse(t)(app.Test(httptest.NewRequest("POST", "/cfg/reload", nil)))
	if resp.StatusCode != 200 {
		t.Fatalf("reload storage config expected 200, got %d", resp.StatusCode)
	}

	resp = mustResponse(t)(app.Test(httptest.NewRequest("POST", "/cfg/validate", nil)))
	if resp.StatusCode != 200 {
		t.Fatalf("validate storage config expected 200, got %d", resp.StatusCode)
	}
}

func TestHealthSubroutes_NoDeps(t *testing.T) {
	s := &Server{dependencies: &ServiceDependencies{}}
	app := fiber.New()
	app.Get("/health/platform-certificates", s.handlePlatformCertificateHealth)
	app.Get("/health/coordination", s.handleCoordinationHealth)

	resp := mustResponse(t)(app.Test(httptest.NewRequest("GET", "/health/platform-certificates", nil)))
	if resp.StatusCode != 200 {
		t.Fatalf("platform-certificates expected 200, got %d", resp.StatusCode)
	}

	resp = mustResponse(t)(app.Test(httptest.NewRequest("GET", "/health/coordination", nil)))
	if resp.StatusCode != 200 {
		t.Fatalf("coordination health expected 200, got %d", resp.StatusCode)
	}
}

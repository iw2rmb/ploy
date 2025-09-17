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

	resp1, err := app.Test(httptest.NewRequest("GET", "/health", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp1 != nil && resp1.Body != nil {
			_ = resp1.Body.Close()
		}
	})
	if resp1.StatusCode != 200 {
		t.Fatalf("storage health expected 200, got %d", resp1.StatusCode)
	}

	resp2, err := app.Test(httptest.NewRequest("GET", "/metrics", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp2 != nil && resp2.Body != nil {
			_ = resp2.Body.Close()
		}
	})
	if resp2.StatusCode != 200 {
		t.Fatalf("storage metrics expected 200, got %d", resp2.StatusCode)
	}

	resp3, err := app.Test(httptest.NewRequest("GET", "/cfg", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp3 != nil && resp3.Body != nil {
			_ = resp3.Body.Close()
		}
	})
	if resp3.StatusCode != 200 {
		t.Fatalf("get storage config expected 200, got %d", resp3.StatusCode)
	}

	resp4, err := app.Test(httptest.NewRequest("POST", "/cfg/reload", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp4 != nil && resp4.Body != nil {
			_ = resp4.Body.Close()
		}
	})
	if resp4.StatusCode != 200 {
		t.Fatalf("reload storage config expected 200, got %d", resp4.StatusCode)
	}

	resp5, err := app.Test(httptest.NewRequest("POST", "/cfg/validate", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp5 != nil && resp5.Body != nil {
			_ = resp5.Body.Close()
		}
	})
	if resp5.StatusCode != 200 {
		t.Fatalf("validate storage config expected 200, got %d", resp5.StatusCode)
	}
}

func TestHealthSubroutes_NoDeps(t *testing.T) {
	s := &Server{dependencies: &ServiceDependencies{}}
	app := fiber.New()
	app.Get("/health/platform-certificates", s.handlePlatformCertificateHealth)
	app.Get("/health/coordination", s.handleCoordinationHealth)

	resp6, err := app.Test(httptest.NewRequest("GET", "/health/platform-certificates", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp6 != nil && resp6.Body != nil {
			_ = resp6.Body.Close()
		}
	})
	if resp6.StatusCode != 200 {
		t.Fatalf("platform-certificates expected 200, got %d", resp6.StatusCode)
	}

	resp7, err := app.Test(httptest.NewRequest("GET", "/health/coordination", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp7 != nil && resp7.Body != nil {
			_ = resp7.Body.Close()
		}
	})
	if resp7.StatusCode != 200 {
		t.Fatalf("coordination health expected 200, got %d", resp7.StatusCode)
	}
}

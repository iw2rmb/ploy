package server

import (
    "bytes"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gofiber/fiber/v2"
    "github.com/iw2rmb/ploy/api/health"
)

func TestDomainRoutes_FallbackHandlers(t *testing.T) {
    s := &Server{app: fiber.New(), config: &ControllerConfig{Port: "18082"}, dependencies: &ServiceDependencies{HealthChecker: health.NewHealthChecker("", "", "")}}
    s.setupRoutes()

    // List domains
    resp1, err := s.app.Test(httptest.NewRequest(http.MethodGet, "/v1/apps/demo/domains", nil))
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp1.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp1.StatusCode) }

    // Add domain
    body := []byte(`{"domain":"demo.example.com"}`)
    req2 := httptest.NewRequest(http.MethodPost, "/v1/apps/demo/domains", bytes.NewReader(body))
    req2.Header.Set("Content-Type", "application/json")
    resp2, err := s.app.Test(req2)
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp2.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp2.StatusCode) }

    // Remove domain
    resp3, err := s.app.Test(httptest.NewRequest(http.MethodDelete, "/v1/apps/demo/domains/demo.example.com", nil))
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp3.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp3.StatusCode) }
}

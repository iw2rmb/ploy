package server

import (
    "net/http/httptest"
    "testing"

    "github.com/gofiber/fiber/v2"
    "github.com/iw2rmb/ploy/api/health"
)

// Test that server-level routing wires health endpoints to the handlers.
func TestServerRoutes_HealthReadyLive(t *testing.T) {
    s := &Server{
        app: fiber.New(),
        dependencies: &ServiceDependencies{
            HealthChecker: health.NewHealthChecker("", "127.0.0.1:8500", "http://127.0.0.1:4646"),
        },
    }
    s.setupRoutes()

    // /health may be 200 or 503 depending on environment; accept both
    resp, err := s.app.Test(httptest.NewRequest("GET", "/health", nil))
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != 200 && resp.StatusCode != 503 {
        t.Fatalf("unexpected /health status: %d", resp.StatusCode)
    }

    // /v1/ready
    resp, err = s.app.Test(httptest.NewRequest("GET", "/v1/ready", nil))
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != 200 && resp.StatusCode != 503 {
        t.Fatalf("unexpected /v1/ready status: %d", resp.StatusCode)
    }

    // /v1/live should be 200
    resp, err = s.app.Test(httptest.NewRequest("GET", "/v1/live", nil))
    if err != nil { t.Fatalf("request failed: %v", err) }
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected /v1/live status: %d", resp.StatusCode)
    }
}


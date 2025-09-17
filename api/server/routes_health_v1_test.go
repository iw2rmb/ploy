package server

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/health"
)

func TestServerRoutes_V1HealthAndMetrics(t *testing.T) {
	s := &Server{
		app: fiber.New(),
		dependencies: &ServiceDependencies{
			HealthChecker: func() *health.HealthChecker {
				hc := health.NewHealthChecker("", "", "")
				hc.SetDependencyChecksEnabled(false)
				return hc
			}(),
		},
	}
	s.setupRoutes()

	// /v1/health may be 200 or 503
	resp, err := s.app.Test(httptest.NewRequest("GET", "/v1/health", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 503 {
		t.Fatalf("unexpected /v1/health status: %d", resp.StatusCode)
	}

	// Root /health/metrics
	resp, err = s.app.Test(httptest.NewRequest("GET", "/health/metrics", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected /health/metrics status: %d", resp.StatusCode)
	}

	// Versioned /v1/health/metrics
	resp, err = s.app.Test(httptest.NewRequest("GET", "/v1/health/metrics", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected /v1/health/metrics status: %d", resp.StatusCode)
	}
}

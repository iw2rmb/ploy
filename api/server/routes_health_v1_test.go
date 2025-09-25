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
				hc := health.NewHealthChecker("", "")
				hc.SetDependencyChecksEnabled(false)
				return hc
			}(),
		},
	}
	s.setupRoutes()

	// /v1/health may be 200 or 503
	resp1, err := s.app.Test(httptest.NewRequest("GET", "/v1/health", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp1 != nil && resp1.Body != nil {
			_ = resp1.Body.Close()
		}
	})
	if resp1.StatusCode != 200 && resp1.StatusCode != 503 {
		t.Fatalf("unexpected /v1/health status: %d", resp1.StatusCode)
	}

	// Root /health/metrics
	resp2, err := s.app.Test(httptest.NewRequest("GET", "/health/metrics", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp2 != nil && resp2.Body != nil {
			_ = resp2.Body.Close()
		}
	})
	if resp2.StatusCode != 200 {
		t.Fatalf("unexpected /health/metrics status: %d", resp2.StatusCode)
	}

	// Versioned /v1/health/metrics
	resp3, err := s.app.Test(httptest.NewRequest("GET", "/v1/health/metrics", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp3 != nil && resp3.Body != nil {
			_ = resp3.Body.Close()
		}
	})
	if resp3.StatusCode != 200 {
		t.Fatalf("unexpected /v1/health/metrics status: %d", resp3.StatusCode)
	}
}

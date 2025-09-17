package health

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestHealthHandlers_Basic(t *testing.T) {
	t.Setenv("PLOY_USE_CONSUL_ENV", "false") // prefer file envstore path for stable results
	app := fiber.New()
	hc := NewHealthChecker("", "127.0.0.1:8500", "http://127.0.0.1:4646")
	hc.SetDependencyChecksEnabled(false)

	app.Get("/health", hc.HealthHandler)
	app.Get("/ready", hc.ReadinessHandler)
	app.Get("/live", hc.LivenessHandler)
	app.Get("/metrics", hc.MetricsHandler)

	// Health likely 503 without storage config
	resp, _ := app.Test(httptest.NewRequest("GET", "/health", nil))
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if resp.StatusCode != 200 && resp.StatusCode != 503 {
		t.Fatalf("unexpected health status: %d", resp.StatusCode)
	}

	// Readiness likely 503 in dev env
	resp, _ = app.Test(httptest.NewRequest("GET", "/ready", nil))
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if resp.StatusCode != 200 && resp.StatusCode != 503 {
		t.Fatalf("unexpected readiness status: %d", resp.StatusCode)
	}

	// Liveness should be 200
	resp, _ = app.Test(httptest.NewRequest("GET", "/live", nil))
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected liveness status: %d", resp.StatusCode)
	}

	// Metrics should be 200
	resp, _ = app.Test(httptest.NewRequest("GET", "/metrics", nil))
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected metrics status: %d", resp.StatusCode)
	}
}

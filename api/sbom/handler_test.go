package sbom

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestSBOMRoutes(t *testing.T) {
	app := fiber.New()
	NewHandler(newMockStorage()).RegisterRoutes(app)

	// Generate
	req := httptest.NewRequest("POST", "/v1/sbom/generate", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for generate, got %d", resp.StatusCode)
	}

	// Analyze
	req = httptest.NewRequest("POST", "/v1/sbom/analyze", nil)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for analyze, got %d", resp.StatusCode)
	}

	// Compliance
	req = httptest.NewRequest("GET", "/v1/sbom/compliance", nil)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for compliance, got %d", resp.StatusCode)
	}

	// Report
	req = httptest.NewRequest("GET", "/v1/sbom/report", nil)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for report, got %d", resp.StatusCode)
	}
}

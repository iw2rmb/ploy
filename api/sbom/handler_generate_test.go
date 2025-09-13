package sbom

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/supply"
)

// mockGenerator implements the minimal generator interface for tests
type mockGenerator struct {
	lastFile string
	lastImg  string
	opts     supply.SBOMGenerationOptions
}

func (m *mockGenerator) GenerateForFile(p string, o supply.SBOMGenerationOptions) error {
	m.lastFile, m.opts = p, o
	return nil
}
func (m *mockGenerator) GenerateForContainer(i string, o supply.SBOMGenerationOptions) error {
	m.lastImg, m.opts = i, o
	return nil
}

func TestGenerateSBOM_UsesGenerator_ForFile(t *testing.T) {
	app := fiber.New()
	mg := &mockGenerator{}
	NewHandlerWithGenerator(mg, newMockStorage()).RegisterRoutes(app)

	body := map[string]any{
		"artifact": "./test-artifact.bin",
		"format":   "spdx-json",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v1/sbom/generate", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if mg.lastFile != "./test-artifact.bin" {
		t.Fatalf("generator not called for file (got %q)", mg.lastFile)
	}
}

func TestGenerateSBOM_UsesGenerator_ForContainer(t *testing.T) {
	app := fiber.New()
	mg := &mockGenerator{}
	NewHandlerWithGenerator(mg, newMockStorage()).RegisterRoutes(app)

	body := map[string]any{
		"artifact": "repo/app:1.2.3",
		"format":   "spdx-json",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v1/sbom/generate", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if mg.lastImg != "repo/app:1.2.3" {
		t.Fatalf("generator not called for image (got %q)", mg.lastImg)
	}
}

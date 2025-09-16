package version

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRegisterRoutes_VersionEndpoints(t *testing.T) {
	app := fiber.New()
	RegisterRoutes(app)

	// /version
	req := httptest.NewRequest("GET", "/version", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if _, ok := body["version"]; !ok {
		t.Fatalf("expected 'version' field in response: %#v", body)
	}

	// /v1/version/detailed
	req2 := httptest.NewRequest("GET", "/v1/version/detailed", nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var det map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&det); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	// Should include at least version and git_commit keys
	if _, ok := det["version"]; !ok {
		t.Fatalf("expected 'version' field in detailed response: %#v", det)
	}
	if _, ok := det["git_commit"]; !ok {
		t.Fatalf("expected 'git_commit' field in detailed response: %#v", det)
	}
}

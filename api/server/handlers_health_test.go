package server

import (
    "encoding/json"
    "net/http/httptest"
    "testing"

    "github.com/gofiber/fiber/v2"
)

func TestHandleCoordinationHealth_Disabled(t *testing.T) {
    s := &Server{dependencies: &ServiceDependencies{CoordinationManager: nil}}
    app := fiber.New()
    app.Get("/health/coordination", s.handleCoordinationHealth)

    req := httptest.NewRequest("GET", "/health/coordination", nil)
    resp, err := app.Test(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("expected 200 status, got %d", resp.StatusCode)
    }
    var body map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
        t.Fatalf("failed to decode json: %v", err)
    }
    if body["status"] != "disabled" {
        t.Fatalf("expected status 'disabled', got %#v", body["status"])
    }
}


package server

import (
	"net/http/httptest"
	"testing"
)

// Recipes routes should be available via internal handlers
func TestRecipesRoutes_InternalHandlers(t *testing.T) {
	srv, err := NewServer(&ControllerConfig{})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	// GET /v1/arf/recipes should respond 200
	req := httptest.NewRequest("GET", "/v1/arf/recipes", nil)
	resp, err := srv.app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

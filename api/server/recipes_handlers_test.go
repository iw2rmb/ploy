package server

import (
    "net/http/httptest"
    "testing"
)

func TestARFRecipesPing_OK(t *testing.T) {
    t.Parallel()
    srv, err := NewServer(&ControllerConfig{})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }
    srv.app.Get("/v1/arf/recipes/ping", srv.handleARFRecipesPing)
    req := httptest.NewRequest("GET", "/v1/arf/recipes/ping", nil)
    resp, err := srv.app.Test(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }
}

func TestARFRecipesList_OK(t *testing.T) {
    t.Parallel()
    srv, err := NewServer(&ControllerConfig{})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }
    srv.app.Get("/v1/arf/recipes", srv.handleARFRecipesList)
    req := httptest.NewRequest("GET", "/v1/arf/recipes?language=java&tag=cleanup", nil)
    resp, err := srv.app.Test(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }
}

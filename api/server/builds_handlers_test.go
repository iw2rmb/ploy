package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestHandleBuildsOptions(t *testing.T) {
	s := &Server{dependencies: &ServiceDependencies{}}
	app := fiber.New()
	app.Options("/v1/apps/:app/builds", s.handleBuildsOptions)

	req := httptest.NewRequest("OPTIONS", "/v1/apps/foo/builds", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	if v := resp.Header.Get("Allow"); v == "" {
		t.Fatalf("expected Allow header, missing")
	}
}

func TestHandleBuildsProbe(t *testing.T) {
	s := &Server{dependencies: &ServiceDependencies{}}
	app := fiber.New()
	app.Post("/v1/apps/:app/builds/probe", s.handleBuildsProbe)

	req := httptest.NewRequest("POST", "/v1/apps/myapp/builds/probe", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["app"].(string) != "myapp" {
		t.Fatalf("expected app=myapp, got %#v", body["app"])
	}
}

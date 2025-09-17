package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
)

func TestEnvHandlers_CRUD(t *testing.T) {
	temp := t.TempDir()
	es := envstore.New(filepath.Join(temp, "env"))
	s := &Server{dependencies: &ServiceDependencies{EnvStore: es}}

	app := fiber.New()
	app.Post("/v1/apps/:app/env", s.handleSetEnvVars)
	app.Get("/v1/apps/:app/env", s.handleGetEnvVars)
	app.Put("/v1/apps/:app/env/:key", s.handleSetEnvVar)
	app.Delete("/v1/apps/:app/env/:key", s.handleDeleteEnvVar)

	// POST set multiple
	body := map[string]string{"A": "1", "B": "2"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v1/apps/myapp/env", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp1, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp1 != nil && resp1.Body != nil {
			_ = resp1.Body.Close()
		}
	})
	if resp1.StatusCode != 200 {
		t.Fatalf("set env vars expected 200, got %d", resp1.StatusCode)
	}

	// GET
	resp2, err := app.Test(httptest.NewRequest("GET", "/v1/apps/myapp/env", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp2 != nil && resp2.Body != nil {
			_ = resp2.Body.Close()
		}
	})
	if resp2.StatusCode != 200 {
		t.Fatalf("get env vars expected 200, got %d", resp2.StatusCode)
	}

	// PUT single
	pb := []byte(`{"value":"3"}`)
	req = httptest.NewRequest("PUT", "/v1/apps/myapp/env/A", bytes.NewReader(pb))
	req.Header.Set("Content-Type", "application/json")
	resp3, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp3 != nil && resp3.Body != nil {
			_ = resp3.Body.Close()
		}
	})
	if resp3.StatusCode != 200 {
		t.Fatalf("put env var expected 200, got %d", resp3.StatusCode)
	}

	// DELETE
	resp4, err := app.Test(httptest.NewRequest("DELETE", "/v1/apps/myapp/env/B", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp4 != nil && resp4.Body != nil {
			_ = resp4.Body.Close()
		}
	})
	if resp4.StatusCode != 200 {
		t.Fatalf("delete env var expected 200, got %d", resp4.StatusCode)
	}

	// Verify underlying file exists
	if _, err := os.Stat(filepath.Join(temp, "env", "myapp.env.json")); err != nil {
		t.Fatalf("expected env file to be created: %v", err)
	}
}

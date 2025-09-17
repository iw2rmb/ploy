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
	resp := mustResponse(t)(app.Test(req))
	if resp.StatusCode != 200 {
		t.Fatalf("set env vars expected 200, got %d", resp.StatusCode)
	}

	// GET
	resp = mustResponse(t)(app.Test(httptest.NewRequest("GET", "/v1/apps/myapp/env", nil)))
	if resp.StatusCode != 200 {
		t.Fatalf("get env vars expected 200, got %d", resp.StatusCode)
	}

	// PUT single
	pb := []byte(`{"value":"3"}`)
	req = httptest.NewRequest("PUT", "/v1/apps/myapp/env/A", bytes.NewReader(pb))
	req.Header.Set("Content-Type", "application/json")
	resp = mustResponse(t)(app.Test(req))
	if resp.StatusCode != 200 {
		t.Fatalf("put env var expected 200, got %d", resp.StatusCode)
	}

	// DELETE
	resp = mustResponse(t)(app.Test(httptest.NewRequest("DELETE", "/v1/apps/myapp/env/B", nil)))
	if resp.StatusCode != 200 {
		t.Fatalf("delete env var expected 200, got %d", resp.StatusCode)
	}

	// Verify underlying file exists
	if _, err := os.Stat(filepath.Join(temp, "env", "myapp.env.json")); err != nil {
		t.Fatalf("expected env file to be created: %v", err)
	}
}

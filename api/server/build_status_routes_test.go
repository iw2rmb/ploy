package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"net/http/httptest"

	"github.com/gofiber/fiber/v2"
)

func TestHandleBuildStatus_ExistingFile_ReturnsJSON(t *testing.T) {
	// Redirect uploadsBaseDir to temp dir
	dir := t.TempDir()
	old := uploadsBaseDir
	uploadsBaseDir = dir
	t.Cleanup(func() { uploadsBaseDir = old })

	// Seed status file
	id := "b-123"
	data := map[string]any{"id": id, "app": "demo", "status": "completed"}
	b, _ := json.Marshal(data)
	if err := os.WriteFile(filepath.Join(dir, id+".json"), b, 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := createMockServer()
	app := fiber.New()
	app.Get("/v1/apps/:app/builds/:id/status", s.handleBuildStatus)

	req := httptest.NewRequest("GET", "/v1/apps/demo/builds/"+id+"/status", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandleBuildStatus_NotFound_Returns404(t *testing.T) {
	dir := t.TempDir()
	old := uploadsBaseDir
	uploadsBaseDir = dir
	t.Cleanup(func() { uploadsBaseDir = old })

	s := createMockServer()
	app := fiber.New()
	app.Get("/v1/apps/:app/builds/:id/status", s.handleBuildStatus)

	req := httptest.NewRequest("GET", "/v1/apps/demo/builds/missing/status", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

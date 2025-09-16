package mods

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestMods_RunMod_MissingConfig(t *testing.T) {
	app := fiber.New()
	h := NewHandler(nil, nil, &kvMem{})
	h.RegisterRoutes(app)

	req := httptest.NewRequest("POST", "/v1/mods", bytes.NewReader([]byte(`{"config":""}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

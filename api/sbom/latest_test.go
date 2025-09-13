package sbom

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestGetLatest_ReturnsPointerFromStorage(t *testing.T) {
	app := fiber.New()
	st := newMockStorage()
	h := NewHandler(st)
	h.RegisterRoutes(app)

	repo := "https://git.example.com/acme/app.git"
	key := latestPointerKey(repo)
	pointer := map[string]any{"key": "mods/exec-123/source.sbom.json"}
	b, _ := json.Marshal(pointer)
	_ = st.Put(context.Background(), key, bytes.NewReader(b))

	req := httptest.NewRequest("GET", "/v1/sbom/latest?repo="+repo, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDownloadSBOM_StreamsFromStorage(t *testing.T) {
	app := fiber.New()
	st := newMockStorage()
	h := NewHandler(st)
	h.RegisterRoutes(app)

	key := "mods/exec-xyz/source.sbom.json"
	data := []byte(`{"sbom":"ok"}`)
	_ = st.Put(context.Background(), key, bytes.NewReader(data))

	req := httptest.NewRequest("GET", "/v1/sbom/download?key="+key, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

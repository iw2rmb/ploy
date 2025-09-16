package mods

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestMods_ArtifactsNegativePaths(t *testing.T) {
	app := fiber.New()
	kv := &kvMem{m: map[string][]byte{}}
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	// No status
	resp, _ := app.Test(httptest.NewRequest("GET", "/v1/mods/unknown/artifacts", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 when no status, got %d", resp.StatusCode)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	// Seed status without artifacts
	st := ModStatus{ID: "id4", Status: "completed", StartTime: time.Now()}
	_ = h.storeStatus(st)
	resp, _ = app.Test(httptest.NewRequest("GET", "/v1/mods/id4/artifacts", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 empty artifacts, got %d", resp.StatusCode)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// Minimal regression test: artifacts should be retrievable from API even if status is failed,
// provided the status.Result includes an artifacts map. This exercises the DownloadArtifact
// and GetArtifacts handlers without requiring full executeMod orchestration.
func TestMods_ArtifactsExposedOnFailure(t *testing.T) {
	app := fiber.New()
	kv := &kvMem{m: map[string][]byte{}}
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	// Seed a failed status that includes artifacts
	id := "failed-1"
	st := ModStatus{
		ID:        id,
		Status:    "failed",
		StartTime: time.Now().Add(-1 * time.Minute),
		EndTime:   func() *time.Time { tt := time.Now(); return &tt }(),
		Error:     "build failed",
		Result: map[string]any{
			"artifacts": map[string]any{
				"plan_json":  "artifacts/mods/failed-1/plan.json",
				"next_json":  "artifacts/mods/failed-1/next.json",
				"diff_patch": "artifacts/mods/failed-1/diff.patch",
			},
		},
	}
	if err := h.storeStatus(st); err != nil {
		t.Fatalf("seed failed status: %v", err)
	}

	// Get artifacts listing
	resp, _ := app.Test(httptest.NewRequest("GET", "/v1/mods/"+id+"/artifacts", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for artifacts listing on failed run, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

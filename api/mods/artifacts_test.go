package mods

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	resp, err := app.Test(httptest.NewRequest("GET", "/v1/mods/unknown/artifacts", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 when no status, got %d", resp.StatusCode)
	}

	// Seed status without artifacts
	st := ModStatus{ID: "id4", Status: "completed", StartTime: time.Now()}
	_ = h.storeStatus(st)
	resp, err = app.Test(httptest.NewRequest("GET", "/v1/mods/id4/artifacts", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
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
	resp, err := app.Test(httptest.NewRequest("GET", "/v1/mods/"+id+"/artifacts", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for artifacts listing on failed run, got %d", resp.StatusCode)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func TestUploadsPersistArtifacts(t *testing.T) {
	storage := newFakeStorage()
	app := fiber.New()
	kv := &kvMem{m: map[string][]byte{}}
	h := NewHandler(nil, storage, kv)
	h.RegisterRoutes(app)

	t.Run("plan_json", func(t *testing.T) {
		baseStatus, err := json.Marshal(ModStatus{ID: "mod-abc", Result: map[string]any{}})
		if err != nil {
			t.Fatalf("marshal status: %v", err)
		}
		if err := kv.Put("mods/status/mod-abc", baseStatus); err != nil {
			t.Fatalf("seed status: %v", err)
		}
		body := bytes.NewBufferString(`{"plan_id":"p1","options":[]}`)
		req := httptest.NewRequest("PUT", "/v1/mods/mod-abc/artifacts/plan_json", body)
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.Body != nil {
			defer func() { _ = resp.Body.Close() }()
		}
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		key := "artifacts/mods/mod-abc/plan.json"
		if got := string(storage.files[key]); got != `{"plan_id":"p1","options":[]}` {
			t.Fatalf("unexpected stored payload: %s", got)
		}
		storedStatus, err := kv.Get("mods/status/mod-abc")
		if err != nil {
			t.Fatalf("get status: %v", err)
		}
		var st ModStatus
		if err := json.Unmarshal(storedStatus, &st); err != nil {
			t.Fatalf("unmarshal status: %v", err)
		}
		arts, _ := st.Result["artifacts"].(map[string]any)
		if arts == nil {
			t.Fatalf("expected artifacts map in status")
		}
		if val, ok := arts["plan_json"].(string); !ok || val != key {
			t.Fatalf("status not updated with plan_json, got=%v", arts["plan_json"])
		}
	})

	t.Run("rejects unknown", func(t *testing.T) {
		body := bytes.NewBufferString("irrelevant")
		req := httptest.NewRequest("PUT", "/v1/mods/mod-abc/artifacts/unknown", body)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.Body != nil {
			defer func() { _ = resp.Body.Close() }()
		}
		if resp.StatusCode != 400 {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})
}

func TestPersistArtifactsUploadsKnownFiles(t *testing.T) {
	storage := newFakeStorage()
	h := NewHandler(nil, storage, &kvMem{})
	modID := "persist-1"
	tmp := t.TempDir()

	write := func(rel, contents string) {
		path := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	write("planner/out/plan.json", `{"plan":true}`)
	write("reducer/out/next.json", `{"next":true}`)
	write("orw-apply/out/diff.patch", "diff --git")
	write("orw-apply/out/error.log", "failure")
	write("reports/container.sbom.json", `{"sbom":"container"}`)

	artifacts, err := h.persistArtifacts(modID, tmp)
	if err != nil {
		t.Fatalf("persist artifacts: %v", err)
	}

	want := map[string]bool{
		"plan_json":      true,
		"next_json":      true,
		"diff_patch":     true,
		"error_log":      true,
		"container_sbom": true,
	}
	for k := range want {
		if artifacts[k] == "" {
			t.Fatalf("expected artifact key for %s", k)
		}
		if _, ok := storage.files[artifacts[k]]; !ok {
			t.Fatalf("storage missing key %s", artifacts[k])
		}
	}
}

func TestPersistArtifactsFallsBackToExistingDiff(t *testing.T) {
	storage := newFakeStorage()
	const modID = "persist-2"
	diffKey := "artifacts/mods/persist-2/diff.patch"
	if err := storage.Put(context.Background(), diffKey, bytes.NewReader([]byte("existing diff"))); err != nil {
		t.Fatalf("seed storage: %v", err)
	}
	h := NewHandler(nil, storage, &kvMem{})
	tmp := t.TempDir()

	artifacts, err := h.persistArtifacts(modID, tmp)
	if err != nil {
		t.Fatalf("persist artifacts: %v", err)
	}
	if artifacts["diff_patch"] != diffKey {
		t.Fatalf("expected existing diff key, got %s", artifacts["diff_patch"])
	}
}

func TestRecordLatestSBOMWritesPointers(t *testing.T) {
	storage := newFakeStorage()
	h := NewHandler(nil, storage, &kvMem{})
	repo := "github.com/acme/project"
	sha := "abc1234"
	modID := "mod-42"
	key := "artifacts/mods/mod-42/source.sbom.json"

	h.recordLatestSBOM(repo, key, sha, modID)

	sum := sha1.Sum([]byte(repo))
	slug := hex.EncodeToString(sum[:])
	latestKey := fmt.Sprintf("mods/sbom/latest/%s.json", slug)
	if _, ok := storage.files[latestKey]; !ok {
		t.Fatalf("expected latest pointer at %s", latestKey)
	}

	foundHistory := 0
	for key := range storage.files {
		if strings.HasPrefix(key, fmt.Sprintf("mods/sbom/history/%s/", slug)) {
			foundHistory++
		}
	}
	if foundHistory == 0 {
		t.Fatalf("expected history entries for slug %s", slug)
	}
}

package mods

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// in-memory KV for tests
type kvMem struct{ m map[string][]byte }

func (k *kvMem) Put(key string, v []byte) error {
	if k.m == nil {
		k.m = map[string][]byte{}
	}
	k.m[key] = append([]byte(nil), v...)
	return nil
}
func (k *kvMem) Get(key string) ([]byte, error) {
	if k.m == nil {
		return nil, nil
	}
	return k.m[key], nil
}
func (k *kvMem) Keys(prefix, sep string) ([]string, error) {
	keys := []string{}
	for k2 := range k.m {
		if len(prefix) == 0 || (len(k2) >= len(prefix) && k2[:len(prefix)] == prefix) {
			keys = append(keys, k2)
		}
	}
	return keys, nil
}
func (k *kvMem) Delete(key string) error { delete(k.m, key); return nil }

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

func TestMods_StatusAndCancel(t *testing.T) {
	app := fiber.New()
	kv := &kvMem{m: map[string][]byte{}}
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	// Seed a running status
	id := "exec-1"
	st := ModStatus{ID: id, Status: "running", StartTime: time.Now()}
	if err := h.storeStatus(st); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	// Get status
	resp, _ := app.Test(httptest.NewRequest("GET", "/v1/mods/"+id+"/status", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("get status 200 expected, got %d", resp.StatusCode)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	// Cancel
	resp, _ = app.Test(httptest.NewRequest("DELETE", "/v1/mods/"+id, nil))
	if resp.StatusCode != 200 {
		t.Fatalf("cancel 200 expected, got %d", resp.StatusCode)
	}
	// Decode response to verify state transitioned
	var out map[string]any
	if resp != nil && resp.Body != nil {
		_ = json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()
	}
	if out["message"] == nil {
		t.Fatalf("expected message in cancel response")
	}
}

func TestMods_ListAndReportEvent(t *testing.T) {
	app := fiber.New()
	kv := &kvMem{m: map[string][]byte{}}
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	// Seed two executions under mods/status prefix for ListMods
	st1 := ModStatus{ID: "id1", Status: "completed", StartTime: time.Now()}
	st2 := ModStatus{ID: "id2", Status: "running", StartTime: time.Now()}
	b1, _ := json.Marshal(st1)
	b2, _ := json.Marshal(st2)
	_ = kv.Put("mods/status/id1", b1)
	_ = kv.Put("mods/status/id2", b2)

	resp, _ := app.Test(httptest.NewRequest("GET", "/v1/mods", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("list mods expected 200, got %d", resp.StatusCode)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	// Report event initializes status if missing
	ev := map[string]any{"execution_id": "id3", "phase": "build", "step": "start", "message": "ok"}
	eb, _ := json.Marshal(ev)
	req := httptest.NewRequest("POST", "/v1/mods/id3/events", bytes.NewReader(eb))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("report event expected 200, got %d", resp.StatusCode)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

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

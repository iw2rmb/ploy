package mods

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	internalmods "github.com/iw2rmb/ploy/internal/mods"
)

func TestMods_RunMod_MissingConfig(t *testing.T) {
	app := fiber.New()
	h := NewHandler(nil, nil, &kvMem{})
	h.RegisterRoutes(app)

	req := httptest.NewRequest("POST", "/v1/mods", bytes.NewReader([]byte(`{"config":""}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestMods_RunMod_InvalidYAML(t *testing.T) {
	app := fiber.New()
	h := NewHandler(nil, nil, &kvMem{})
	h.RegisterRoutes(app)

	req := httptest.NewRequest("POST", "/v1/mods", bytes.NewReader([]byte(`{"config":":bad yaml"}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestMods_RunMod_InvalidConfigData(t *testing.T) {
	app := fiber.New()
	h := NewHandler(nil, nil, &kvMem{})
	h.RegisterRoutes(app)

	payload := `{"config_data": {"version":"v1","id":"demo","target_repo":"repo","base_ref":"main","steps":[]}}`
	req := httptest.NewRequest("POST", "/v1/mods", bytes.NewReader([]byte(payload)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestExecuteModRecordsErrorOnInvalidConfig(t *testing.T) {
	kv := &kvMem{}
	h := NewHandler(nil, nil, kv)

	h.executeMod("mod-invalid", &internalmods.ModConfig{}, false)

	st, err := h.getStatus("mod-invalid")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if st.Status != "failed" {
		t.Fatalf("expected failed status, got %s", st.Status)
	}
	if st.Error == "" {
		t.Fatalf("expected error message to be recorded")
	}
}

func TestRecordErrorPreservesExistingData(t *testing.T) {
	kv := &kvMem{}
	h := NewHandler(nil, nil, kv)

	start := time.Now().Add(-time.Minute)
	existing := ModStatus{
		ID:        "mod-record",
		Status:    "running",
		StartTime: start,
		Phase:     "planner",
		Steps: []ModStepStatus{{
			Step:    "plan",
			Phase:   "planner",
			Message: "starting",
			Time:    start,
		}},
	}
	if err := h.storeStatus(existing); err != nil {
		t.Fatalf("store status: %v", err)
	}

	h.recordError("mod-record", fmt.Errorf("boom"))

	st, err := h.getStatus("mod-record")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if st.Status != "failed" {
		t.Fatalf("expected failed status, got %s", st.Status)
	}
	if st.Error != "boom" {
		t.Fatalf("expected error to be recorded, got %s", st.Error)
	}
	if len(st.Steps) != 1 || st.Steps[0].Step != "plan" {
		t.Fatalf("expected steps to be preserved, got %#v", st.Steps)
	}
	if st.EndTime == nil {
		t.Fatalf("expected end time to be set")
	}
}

package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestGetModStatusEnrichesRunning(t *testing.T) {
	t.Setenv("PLOY_MODS_OVERDUE", "10s")
	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	start := time.Now().Add(-15 * time.Second).Add(-500 * time.Millisecond)
	status := ModStatus{ID: "mod-running", Status: "running", StartTime: start}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/mods/mod-running/status", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got ModStatus
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Duration == "" {
		t.Fatalf("expected duration to be populated")
	}
	dur, err := time.ParseDuration(got.Duration)
	if err != nil {
		t.Fatalf("duration parse: %v", err)
	}
	if dur < 10*time.Second {
		t.Fatalf("expected duration >=10s, got %s", got.Duration)
	}
	if !got.Overdue {
		t.Fatalf("expected overdue flag to be true")
	}
}

func TestGetModStatusComputesTerminalDuration(t *testing.T) {
	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	start := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	end := start.Add(5 * time.Minute)
	status := ModStatus{ID: "mod-done", Status: "completed", StartTime: start, EndTime: &end}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/mods/mod-done/status", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got ModStatus
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	dur, err := time.ParseDuration(got.Duration)
	if err != nil {
		t.Fatalf("duration parse: %v", err)
	}
	expected := end.Sub(start)
	if dur != expected {
		t.Fatalf("expected %s, got %s", expected, got.Duration)
	}
}

func TestCancelModTransitionsToCancelled(t *testing.T) {
	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	status := ModStatus{ID: "cancel-me", Status: "running", StartTime: time.Now().Add(-1 * time.Minute)}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodDelete, "/v1/mods/cancel-me", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}

	updated, err := h.getStatus("cancel-me")
	if err != nil {
		t.Fatalf("load status: %v", err)
	}
	if updated.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %s", updated.Status)
	}
	if updated.EndTime == nil {
		t.Fatalf("expected end time to be set")
	}
}

func TestCancelModRejectsNonRunning(t *testing.T) {
	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	status := ModStatus{ID: "already-done", Status: "completed", StartTime: time.Now().Add(-2 * time.Minute)}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodDelete, "/v1/mods/already-done", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestListModsAggregatesStatuses(t *testing.T) {
	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	statuses := []ModStatus{
		{ID: "mod-a", Status: "completed", StartTime: time.Now().Add(-2 * time.Hour)},
		{ID: "mod-b", Status: "running", StartTime: time.Now().Add(-5 * time.Minute)},
	}
	for _, st := range statuses {
		if err := h.storeStatus(st); err != nil {
			t.Fatalf("seed status: %v", err)
		}
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/mods", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Executions []ModStatus `json:"executions"`
		Count      int         `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count != 2 {
		t.Fatalf("expected count 2, got %d", payload.Count)
	}
	if len(payload.Executions) != 2 {
		t.Fatalf("expected 2 executions, got %d", len(payload.Executions))
	}
}

func TestReportEventAppendsStepAndLastJob(t *testing.T) {
	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	payload := map[string]any{
		"phase":    "planner",
		"step":     "start",
		"level":    "info",
		"message":  "planning",
		"job_name": "planner-job",
		"alloc_id": "alloc-123",
		"ts":       time.Now().Add(-30 * time.Second),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod-event/events", bytes.NewReader(body))
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

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	stored, err := h.getStatus("mod-event")
	if err != nil {
		t.Fatalf("load status: %v", err)
	}
	if stored.Phase != "planner" {
		t.Fatalf("expected phase planner, got %s", stored.Phase)
	}
	if len(stored.Steps) != 1 {
		t.Fatalf("expected one step, got %d", len(stored.Steps))
	}
	if stored.Steps[0].Step != "start" || stored.Steps[0].Message != "planning" {
		t.Fatalf("unexpected step payload: %+v", stored.Steps[0])
	}
	if stored.LastJob == nil || stored.LastJob.JobName != "planner-job" {
		t.Fatalf("expected last job to be set")
	}
}

func TestArtifactEndpointsListAndStreamDiff(t *testing.T) {
	storage := newFakeStorage()
	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, storage, kv)
	h.RegisterRoutes(app)

	id := "mod-artifacts"
	diffKey := fmt.Sprintf("artifacts/mods/%s/diff.patch", id)
	if err := storage.Put(context.Background(), diffKey, bytes.NewReader([]byte("diff --git"))); err != nil {
		t.Fatalf("seed storage: %v", err)
	}
	status := ModStatus{
		ID:        id,
		Status:    "failed",
		StartTime: time.Now().Add(-2 * time.Minute),
		EndTime:   func() *time.Time { now := time.Now(); return &now }(),
		Result: map[string]any{
			"artifacts": map[string]any{
				"diff_patch": diffKey,
			},
		},
	}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/mods/"+id+"/artifacts", nil))
	if err != nil {
		t.Fatalf("artifacts request failed: %v", err)
	}
	defer func() {
		if listResp != nil && listResp.Body != nil {
			_ = listResp.Body.Close()
		}
	}()

	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listPayload map[string]map[string]string
	if err := json.NewDecoder(listResp.Body).Decode(&listPayload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if listPayload["artifacts"]["diff_patch"] != diffKey {
		t.Fatalf("expected diff key %s", diffKey)
	}

	streamReq := httptest.NewRequest(http.MethodGet, "/v1/mods/"+id+"/artifacts/diff_patch", nil)
	streamResp, err := app.Test(streamReq)
	if err != nil {
		t.Fatalf("stream request failed: %v", err)
	}
	defer func() {
		if streamResp != nil && streamResp.Body != nil {
			_ = streamResp.Body.Close()
		}
	}()

	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", streamResp.StatusCode)
	}
	if disp := streamResp.Header.Get("Content-Disposition"); !strings.Contains(disp, "diff_patch") {
		t.Fatalf("expected diff filename in content-disposition, got %q", disp)
	}
	bodyBytes, _ := io.ReadAll(streamResp.Body)
	if string(bodyBytes) != "diff --git" {
		t.Fatalf("unexpected diff content: %s", string(bodyBytes))
	}
}

func TestDownloadArtifactRejectsInvalidKey(t *testing.T) {
	storage := newFakeStorage()
	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, storage, kv)
	h.RegisterRoutes(app)

	id := "bad-artifact"
	status := ModStatus{
		ID:        id,
		Status:    "failed",
		StartTime: time.Now(),
		Result: map[string]any{
			"artifacts": map[string]any{
				"diff_patch": fmt.Sprintf("artifacts/mods/%s/../../escape.patch", id),
			},
		},
	}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/mods/"+id+"/artifacts/diff_patch", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

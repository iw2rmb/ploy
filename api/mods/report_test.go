package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	internalmods "github.com/iw2rmb/ploy/internal/mods"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
)

func TestGetModReportJSON(t *testing.T) {
	kv := &kvMem{}
	storage := newFakeStorage()
	h := NewHandler(nil, storage, kv)
	app := fiber.New()
	h.RegisterRoutes(app)

	start := time.Date(2025, 1, 2, 15, 4, 5, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	report := internalmods.ModReport{
		RepoName:   "https://git.example.com/org/service.git",
		WorkflowID: "report-json",
		MRURL:      "https://git.example.com/org/service/merge_requests/7",
		StartedAt:  start,
		EndedAt:    end,
		Duration:   end.Sub(start),
		HappyPath: []internalmods.ReportStep{{
			ID:      "clone",
			Type:    "system",
			Message: "Cloned repository",
		}},
		StepTree: []internalmods.ReportStepNode{{
			ID:      "clone",
			Type:    "system",
			Success: true,
			Message: "Cloned repository",
		}},
	}

	status := ModStatus{ID: "mod-report-json", Status: "completed", StartTime: start, EndTime: &end}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("store status: %v", err)
	}

	payload, _ := json.Marshal(report)
	if err := storage.Put(context.Background(), reportStorageKey("mod-report-json"), bytes.NewReader(payload), internalStorage.WithContentType("application/json")); err != nil {
		t.Fatalf("put report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/mod-report-json/report", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got internalmods.ModReport
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.MRURL != report.MRURL {
		t.Fatalf("expected MR URL %q, got %q", report.MRURL, got.MRURL)
	}
	if len(got.HappyPath) != 1 || got.HappyPath[0].ID != "clone" {
		t.Fatalf("expected happy path clone step, got %+v", got.HappyPath)
	}
}

func TestGetModReportMarkdown(t *testing.T) {
	kv := &kvMem{}
	storage := newFakeStorage()
	h := NewHandler(nil, storage, kv)
	app := fiber.New()
	h.RegisterRoutes(app)

	start := time.Date(2025, 3, 4, 10, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Minute)
	report := internalmods.ModReport{
		RepoName:   "https://git.example.com/org/service.git",
		WorkflowID: "report-md",
		StartedAt:  start,
		EndedAt:    end,
		Duration:   end.Sub(start),
		HappyPath: []internalmods.ReportStep{{
			ID:      "apply",
			Type:    string(internalmods.StepTypeORWApply),
			Message: "Applied ORW diff",
			Diff:    &internalmods.ReportDiff{Content: "diff --git a/file b/file\n+change"},
		}},
		StepTree: []internalmods.ReportStepNode{{
			ID:      "apply",
			Type:    string(internalmods.StepTypeORWApply),
			Success: true,
			Message: "Applied ORW diff",
		}},
	}

	status := ModStatus{ID: "mod-report-md", Status: "completed", StartTime: start, EndTime: &end}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("store status: %v", err)
	}

	payload, _ := json.Marshal(report)
	if err := storage.Put(context.Background(), reportStorageKey("mod-report-md"), bytes.NewReader(payload), internalStorage.WithContentType("application/json")); err != nil {
		t.Fatalf("put report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/mod-report-md/report?format=markdown", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	content := string(body)
	if !strings.Contains(content, "```diff") {
		t.Fatalf("expected diff fence in markdown response, got:\n%s", content)
	}
	if !strings.Contains(content, "report-md") {
		t.Fatalf("expected workflow ID in markdown response, got:\n%s", content)
	}
}

func TestGetModReportMissingReportReturnsNotFound(t *testing.T) {
	kv := &kvMem{}
	storage := newFakeStorage()
	h := NewHandler(nil, storage, kv)
	app := fiber.New()
	h.RegisterRoutes(app)

	status := ModStatus{ID: "mod-no-report", Status: "completed", StartTime: time.Now()}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("store status: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/mod-no-report/report", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetModReportStorageDisabled(t *testing.T) {
	kv := &kvMem{}
	h := NewHandler(nil, nil, kv)
	app := fiber.New()
	h.RegisterRoutes(app)

	status := ModStatus{ID: "mod-no-storage", Status: "completed", StartTime: time.Now()}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("store status: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/mod-no-storage/report", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

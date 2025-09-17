package mods

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestStreamLogsFollowFalse(t *testing.T) {
	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	status := ModStatus{
		ID:        "log-follow-false",
		Status:    "running",
		Phase:     "planner",
		Duration:  "1m0s",
		StartTime: time.Now().Add(-time.Minute),
		Steps: []ModStepStatus{{
			Step:    "plan",
			Phase:   "planner",
			Message: "starting",
			Time:    time.Now(),
		}},
	}
	if err := h.storeStatus(status); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/log-follow-false/logs?follow=false", nil)
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
	body, _ := io.ReadAll(resp.Body)
	stream := string(body)
	if !strings.Contains(stream, "event: init") {
		t.Fatalf("expected init event, got %q", stream)
	}
	if !strings.Contains(stream, "event: step") {
		t.Fatalf("expected step event, got %q", stream)
	}
	if !strings.Contains(stream, "event: end") {
		t.Fatalf("expected end event, got %q", stream)
	}
}

func TestTailAllocLogsUsesWrapper(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "nomad-job-manager.sh")
	scriptBody := "#!/bin/sh\nif [ \"$1\" = \"logs\" ]; then\n  echo 'preview line 1'\n  exit 0\nfi\nexit 0\n"
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("NOMAD_JOB_MANAGER", script)

	out := tailAllocLogs("alloc-1", "planner", 5)
	if !strings.Contains(out, "preview line 1") {
		t.Fatalf("expected log preview, got %q", out)
	}
}

func TestTailAllocLogsMissingManager(t *testing.T) {
	t.Setenv("NOMAD_JOB_MANAGER", filepath.Join(t.TempDir(), "missing.sh"))
	if got := tailAllocLogs("alloc-1", "planner", 5); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestTaskForJob(t *testing.T) {
	cases := map[string]string{
		"mod-planner-1": "planner",
		"MOD-REDUCER":   "reducer",
		"mod-llm-exec":  "llm-exec",
		"orw-apply-123": "openrewrite-apply",
		"unknown-job":   "api",
	}
	for job, want := range cases {
		if got := taskForJob(job); got != want {
			t.Fatalf("taskForJob(%s)=%s want %s", job, got, want)
		}
	}
}

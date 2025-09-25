package mods

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestReportEventPublishesModsTelemetry(t *testing.T) {
	h := NewHandler(nil, newFakeStorage(), &kvMem{})

	var (
		mu      sync.Mutex
		records []ModEvent
	)
	h.SetEventPublisher(func(ctx context.Context, ev ModEvent) {
		if ctx == nil {
			t.Fatalf("expected non-nil context")
		}
		mu.Lock()
		defer mu.Unlock()
		records = append(records, ev)
	})

	app := fiber.New()
	app.Post("/v1/mods/:id/events", h.ReportEvent)

	ev := ModEvent{Phase: "plan", Step: "init", Level: "info", Message: "starting"}
	body, _ := json.Marshal(ev)
	req := httptest.NewRequest("POST", "/v1/mods/mod-1/events", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(records) != 1 {
		t.Fatalf("expected 1 event, got %d", len(records))
	}
	if records[0].ModID != "mod-1" {
		t.Fatalf("expected mod id propagated, got %s", records[0].ModID)
	}
	if records[0].Phase != "plan" || records[0].Step != "init" {
		t.Fatalf("unexpected event payload: %+v", records[0])
	}
}

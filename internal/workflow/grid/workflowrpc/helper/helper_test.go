package helper

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/grid/workflowrpc"
)

func TestHelperSubmitAddsAuthorizationHeader(t *testing.T) {
	ticket := contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}
	stage := workflowrpc.Stage{Name: "mods-plan", Kind: "mods-plan", Lane: "node-wasm", Job: workflowrpc.JobSpec{Image: "registry.dev/node:20", Command: []string{"npm", "test"}}}

	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "stage": stage})
	}))
	defer server.Close()

	helper, err := New(Options{Endpoint: server.URL, HTTPClient: server.Client(), BearerToken: "token-123"})
	if err != nil {
		t.Fatalf("new helper: %v", err)
	}

	_, err = helper.Submit(context.Background(), workflowrpc.SubmitRequest{SchemaVersion: contracts.SchemaVersion, Ticket: ticket, Stage: stage})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if capturedAuth != "Bearer token-123" {
		t.Fatalf("expected bearer token header, got %q", capturedAuth)
	}
}

func TestHelperSubmitRetriesRetryableErrors(t *testing.T) {
	attempts := 0
	stage := workflowrpc.Stage{Name: "build", Kind: "mods-build", Lane: "go", Job: workflowrpc.JobSpec{Image: "registry.dev/build:latest", Command: []string{"/bin/build"}}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "stage": stage})
	}))
	defer server.Close()

	helper, err := New(Options{Endpoint: server.URL, HTTPClient: server.Client(), Retries: 3, Backoff: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("new helper: %v", err)
	}

	resp, err := helper.Submit(context.Background(), workflowrpc.SubmitRequest{SchemaVersion: contracts.SchemaVersion, Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-1", Tenant: "acme", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}, Stage: stage})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if resp.Status != "completed" {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestHelperSubmitHonoursContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	helper, err := New(Options{Endpoint: server.URL, HTTPClient: server.Client(), Retries: 5, Backoff: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("new helper: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = helper.Submit(ctx, workflowrpc.SubmitRequest{SchemaVersion: contracts.SchemaVersion, Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-1", Tenant: "acme", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

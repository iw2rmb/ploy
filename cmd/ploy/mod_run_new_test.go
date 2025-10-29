package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

func TestExecuteModRunSubmitsTicket(t *testing.T) {
	var received modsapi.TicketSubmitRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
    if r.URL.Path != "/v1/mods" {
        t.Fatalf("unexpected path %s", r.URL.Path)
    }
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		resp := modsapi.TicketSubmitResponse{Ticket: modsapi.TicketSummary{TicketID: received.TicketID, State: modsapi.TicketStatePending}}
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	t.Setenv(controlPlaneURLEnv, server.URL)

	buf := &bytes.Buffer{}
	args := []string{"--tenant", "acme", "--ticket", "mods-test", "--repo-url", "https://example.com/repo.git", "--repo-target-ref", "feature"}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}

	if received.TicketID != "mods-test" {
		t.Fatalf("expected ticket id mods-test, got %s", received.TicketID)
	}
	if received.Tenant != "acme" {
		t.Fatalf("expected tenant acme, got %s", received.Tenant)
	}
	if received.Repository != "https://example.com/repo.git" {
		t.Fatalf("unexpected repository: %s", received.Repository)
	}
	if received.Metadata["repo_target_ref"] != "feature" {
		t.Fatalf("expected repo target metadata, got %v", received.Metadata)
	}
	if len(received.Stages) != 6 {
		t.Fatalf("expected 6 stages, got %d", len(received.Stages))
	}
	output := buf.String()
	if !strings.Contains(output, "Mods ticket mods-test submitted") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestExecuteModRunGeneratesTicket(t *testing.T) {
	var received modsapi.TicketSubmitRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		resp := modsapi.TicketSubmitResponse{Ticket: modsapi.TicketSummary{TicketID: received.TicketID, State: modsapi.TicketStatePending}}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv(controlPlaneURLEnv, server.URL)

	if err := executeModRun([]string{"--tenant", "acme"}, io.Discard); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}
	if received.TicketID == "" {
		t.Fatalf("expected generated ticket id")
	}
	if !strings.HasPrefix(received.TicketID, "mods-") {
		t.Fatalf("expected mods- prefix, got %s", received.TicketID)
	}
}

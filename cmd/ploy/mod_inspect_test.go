package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

func TestModInspectPrintsSummary(t *testing.T) {
	t.Helper()
	ticket := "ticket-11"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/mods/"+ticket {
			_ = json.NewEncoder(w).Encode(modsapi.TicketStatusResponse{Ticket: modsapi.TicketSummary{TicketID: ticket, State: modsapi.TicketStateRunning}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "inspect", ticket}, buf)
	if err != nil {
		t.Fatalf("mod inspect error: %v", err)
	}
	out := buf.String()
	if out == "" || !bytes.Contains([]byte(out), []byte(ticket)) {
		t.Fatalf("expected summary output to include ticket id; got %q", out)
	}
}

func TestModInspectShowsMRURL(t *testing.T) {
	t.Helper()
	ticket := "ticket-mr-123"
	mrURL := "https://gitlab.com/example/repo/-/merge_requests/42"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/mods/"+ticket {
			resp := modsapi.TicketStatusResponse{
				Ticket: modsapi.TicketSummary{
					TicketID: ticket,
					State:    modsapi.TicketStateSucceeded,
					Metadata: map[string]string{"mr_url": mrURL},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "inspect", ticket}, buf)
	if err != nil {
		t.Fatalf("mod inspect error: %v", err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("MR: "+mrURL)) {
		t.Fatalf("expected output to include MR URL; got %q", out)
	}
}

func TestModInspectOmitsMRURLWhenMissing(t *testing.T) {
	t.Helper()
	ticket := "ticket-no-mr"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/mods/"+ticket {
			resp := modsapi.TicketStatusResponse{
				Ticket: modsapi.TicketSummary{
					TicketID: ticket,
					State:    modsapi.TicketStateSucceeded,
					// No metadata or empty metadata.
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "inspect", ticket}, buf)
	if err != nil {
		t.Fatalf("mod inspect error: %v", err)
	}
	out := buf.String()
	if bytes.Contains([]byte(out), []byte("MR:")) {
		t.Fatalf("did not expect MR line when metadata missing; got %q", out)
	}
}

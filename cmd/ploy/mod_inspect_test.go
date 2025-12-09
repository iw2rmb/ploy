package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

func TestModInspectPrintsSummary(t *testing.T) {
	t.Helper()
	runID := "run-11"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/mods/"+runID {
			// Return RunSummary directly — the canonical response shape.
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateRunning,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "inspect", runID}, buf)
	if err != nil {
		t.Fatalf("mod inspect error: %v", err)
	}
	out := buf.String()
	if out == "" || !bytes.Contains([]byte(out), []byte(runID)) {
		t.Fatalf("expected summary output to include run id; got %q", out)
	}
}

func TestModInspectShowsMRURL(t *testing.T) {
	t.Helper()
	runID := "run-mr-123"
	mrURL := "https://gitlab.com/example/repo/-/merge_requests/42"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/mods/"+runID {
			// Return RunSummary directly — the canonical response shape.
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID:    domaintypes.RunID(runID),
				State:    modsapi.RunStateSucceeded,
				Metadata: map[string]string{"mr_url": mrURL},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "inspect", runID}, buf)
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
	runID := "run-no-mr"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/mods/"+runID {
			// Return RunSummary directly — the canonical response shape.
			// No metadata or empty metadata.
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "inspect", runID}, buf)
	if err != nil {
		t.Fatalf("mod inspect error: %v", err)
	}
	out := buf.String()
	if bytes.Contains([]byte(out), []byte("MR:")) {
		t.Fatalf("did not expect MR line when metadata missing; got %q", out)
	}
}

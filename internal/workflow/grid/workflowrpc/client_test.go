package workflowrpc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

type submitRequestPayload struct {
	SchemaVersion string                   `json:"schema_version"`
	Ticket        contracts.WorkflowTicket `json:"ticket"`
	Stage         Stage                    `json:"stage"`
}

func TestClientSubmitSuccess(t *testing.T) {
	ticket := contracts.WorkflowTicket{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      "ticket-123",
		Tenant:        "acme",
		Manifest:      contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"},
	}

	manifest := manifests.Compilation{
		Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26", Summary: "sample"},
	}

	stage := Stage{
		Name:        "mods-plan",
		Kind:        "mods-plan",
		Lane:        "node-wasm",
		CacheKey:    "node/cache",
		Constraints: Constraints{Manifest: manifest},
		Aster: Aster{
			Enabled: true,
			Toggles: []string{"plan"},
			Bundles: []aster.Metadata{{BundleID: "bundle-1", Stage: "mods-plan", Toggle: "plan", ArtifactCID: "bafy123", Digest: "sha256:abc"}},
		},
		Job: JobSpec{
			Image:   "registry.dev/node:20",
			Command: []string{"npm", "test"},
			Env: map[string]string{
				"NODE_ENV": "test",
			},
			Resources: Resources{CPU: "2000m", Memory: "4Gi"},
			Metadata: map[string]string{
				"lane":     "node-wasm",
				"priority": "standard",
			},
		},
	}

	var captured submitRequestPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != submitPath {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("unexpected content type: %s", ct)
		}
		defer func() {
			_ = r.Body.Close()
		}()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "completed",
			"message":   "ok",
			"retryable": false,
			"stage":     stage,
			"artifacts": []map[string]any{{
				"name":         "mods-plan",
				"artifact_cid": "cid-mods-plan",
				"digest":       "sha256:modsplan",
				"media_type":   "application/tar+zst",
			}},
		})
	}))
	defer server.Close()

	client, err := NewClient(Options{Endpoint: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.Submit(context.Background(), SubmitRequest{SchemaVersion: contracts.SchemaVersion, Ticket: ticket, Stage: stage})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if resp.Status != "completed" {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	if resp.Message != "ok" {
		t.Fatalf("unexpected message: %s", resp.Message)
	}
	if resp.Retryable {
		t.Fatal("expected non-retryable")
	}
	if len(resp.Artifacts) != 1 {
		t.Fatalf("expected artifact manifest, got %#v", resp.Artifacts)
	}
	art := resp.Artifacts[0]
	if art.ArtifactCID != "cid-mods-plan" || art.Digest != "sha256:modsplan" {
		t.Fatalf("unexpected artifact manifest: %#v", art)
	}

	if captured.SchemaVersion != contracts.SchemaVersion {
		t.Fatalf("unexpected schema version: %s", captured.SchemaVersion)
	}
	if captured.Ticket.TicketID != ticket.TicketID {
		t.Fatalf("ticket mismatch: %s", captured.Ticket.TicketID)
	}
	if captured.Stage.Name != stage.Name {
		t.Fatalf("stage name mismatch: %s", captured.Stage.Name)
	}
	if captured.Stage.Kind != stage.Kind {
		t.Fatalf("stage kind mismatch: %s", captured.Stage.Kind)
	}
	if captured.Stage.CacheKey != stage.CacheKey {
		t.Fatalf("cache key mismatch: %s", captured.Stage.CacheKey)
	}
	if captured.Stage.Aster.Enabled != stage.Aster.Enabled {
		t.Fatalf("aster enabled mismatch: %v", captured.Stage.Aster.Enabled)
	}
	if captured.Stage.Job.Image != stage.Job.Image {
		t.Fatalf("job image mismatch: %s", captured.Stage.Job.Image)
	}
	if len(captured.Stage.Job.Command) != len(stage.Job.Command) {
		t.Fatalf("job command length mismatch: %#v", captured.Stage.Job.Command)
	}
	if captured.Stage.Job.Env["NODE_ENV"] != "test" {
		t.Fatalf("job env mismatch: %#v", captured.Stage.Job.Env)
	}
	if captured.Stage.Job.Resources.CPU != stage.Job.Resources.CPU {
		t.Fatalf("job resource cpu mismatch: %s", captured.Stage.Job.Resources.CPU)
	}
	if captured.Stage.Job.Metadata["lane"] != "node-wasm" {
		t.Fatalf("job metadata missing lane: %#v", captured.Stage.Job.Metadata)
	}
}

func TestClientSubmitHandlesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(Options{Endpoint: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Submit(context.Background(), SubmitRequest{SchemaVersion: contracts.SchemaVersion, Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-1", Tenant: "acme", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}})
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", httpErr.StatusCode)
	}
}

func TestClientSubmitHandlesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	client, err := NewClient(Options{Endpoint: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Submit(context.Background(), SubmitRequest{SchemaVersion: contracts.SchemaVersion, Ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-1", Tenant: "acme", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}})
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestNewClientValidatesEndpoint(t *testing.T) {
	_, err := NewClient(Options{})
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}

	_, err = NewClient(Options{Endpoint: "://invalid"})
	if err == nil {
		t.Fatal("expected error for invalid endpoint")
	}
}

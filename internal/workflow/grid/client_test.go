package grid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

type requestPayload struct {
	SchemaVersion string                   `json:"schema_version"`
	Ticket        contracts.WorkflowTicket `json:"ticket"`
	Stage         stagePayload             `json:"stage"`
}

type stagePayload struct {
	Name         string             `json:"name"`
	Kind         string             `json:"kind"`
	Lane         string             `json:"lane"`
	Dependencies []string           `json:"dependencies"`
	CacheKey     string             `json:"cache_key"`
	Constraints  constraintsPayload `json:"constraints"`
	Aster        asterPayload       `json:"aster"`
}

type constraintsPayload struct {
	Manifest manifests.Compilation `json:"manifest"`
}

type asterPayload struct {
	Enabled bool             `json:"enabled"`
	Toggles []string         `json:"toggles"`
	Bundles []aster.Metadata `json:"bundles"`
}

func TestClientExecuteStageSuccess(t *testing.T) {
	ticket := contracts.WorkflowTicket{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      "ticket-123",
		Tenant:        "acme",
		Manifest:      contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"},
	}

	manifest := manifests.Compilation{
		Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26", Summary: "sample"},
		Lanes:    manifests.LaneSet{Required: []manifests.Lane{{Name: "go-native"}}},
		Aster:    manifests.AsterSet{Required: []string{"plan"}},
	}

	stage := runner.Stage{
		Name:         "build",
		Kind:         runner.StageKindBuild,
		Lane:         "go-native",
		Dependencies: []string{"mods"},
		CacheKey:     "go-native/cache@manifest=2025-09-26@aster=plan",
		Constraints:  runner.StageConstraints{Manifest: manifest},
		Aster: runner.StageAster{
			Enabled: true,
			Toggles: []string{"plan"},
			Bundles: []aster.Metadata{{BundleID: "bundle-1", Stage: "build", Toggle: "plan", ArtifactCID: "bafy123", Digest: "sha256:abc"}},
		},
	}

	var captured requestPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/workflow/stages" {
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
		})
	}))
	defer server.Close()

	client, err := NewClient(Options{Endpoint: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	outcome, err := client.ExecuteStage(context.Background(), ticket, stage, "/tmp/work")
	if err != nil {
		t.Fatalf("execute stage: %v", err)
	}
	if outcome.Status != runner.StageStatusCompleted {
		t.Fatalf("unexpected status: %s", outcome.Status)
	}
	if outcome.Message != "ok" {
		t.Fatalf("unexpected message: %s", outcome.Message)
	}
	if outcome.Retryable {
		t.Fatal("expected non-retryable")
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
	if captured.Stage.Kind != string(stage.Kind) {
		t.Fatalf("stage kind mismatch: %s", captured.Stage.Kind)
	}
	if captured.Stage.Lane != stage.Lane {
		t.Fatalf("lane mismatch: %s", captured.Stage.Lane)
	}
	if captured.Stage.CacheKey != stage.CacheKey {
		t.Fatalf("cache key mismatch: %s", captured.Stage.CacheKey)
	}
	if captured.Stage.Aster.Enabled != stage.Aster.Enabled {
		t.Fatalf("aster enabled mismatch: %v", captured.Stage.Aster.Enabled)
	}
	if len(captured.Stage.Aster.Bundles) != len(stage.Aster.Bundles) {
		t.Fatalf("expected %d bundles, got %d", len(stage.Aster.Bundles), len(captured.Stage.Aster.Bundles))
	}
	if captured.Stage.Constraints.Manifest.Manifest.Name != manifest.Manifest.Name {
		t.Fatalf("manifest name mismatch: %s", captured.Stage.Constraints.Manifest.Manifest.Name)
	}

	invoker, ok := any(client).(interface {
		Invocations() []runner.StageInvocation
	})
	if !ok {
		t.Fatal("client does not expose invocation reporter")
	}
	invocations := invoker.Invocations()
	if len(invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(invocations))
	}
	if invocations[0].TicketID != ticket.TicketID {
		t.Fatalf("invocation ticket mismatch: %s", invocations[0].TicketID)
	}
	if invocations[0].Stage.Name != stage.Name {
		t.Fatalf("invocation stage mismatch: %s", invocations[0].Stage.Name)
	}
}

func TestClientExecuteStageHandlesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(Options{Endpoint: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.ExecuteStage(context.Background(), contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-1", Tenant: "acme", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}, runner.Stage{Name: "mods", Kind: runner.StageKindMods, Lane: "node-wasm", Constraints: runner.StageConstraints{Manifest: manifests.Compilation{Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"}}}}, "/tmp")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

func TestClientExecuteStageHandlesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	client, err := NewClient(Options{Endpoint: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.ExecuteStage(context.Background(), contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-1", Tenant: "acme", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}, runner.Stage{Name: "mods", Kind: runner.StageKindMods, Lane: "node-wasm", Constraints: runner.StageConstraints{Manifest: manifests.Compilation{Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"}}}}, "/tmp")
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

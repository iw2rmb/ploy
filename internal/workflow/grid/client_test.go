package grid

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/grid/workflowrpc"
	rpcHelper "github.com/iw2rmb/ploy/internal/workflow/grid/workflowrpc/helper"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

const buildGateStageName = "build-gate"

type fakeWorkflowRPC struct {
	requests []workflowrpc.SubmitRequest
	resp     workflowrpc.SubmitResponse
	err      error
}

func (f *fakeWorkflowRPC) Submit(ctx context.Context, req workflowrpc.SubmitRequest) (workflowrpc.SubmitResponse, error) {
	_ = ctx
	f.requests = append(f.requests, req)
	if f.err != nil {
		return workflowrpc.SubmitResponse{}, f.err
	}
	return f.resp, nil
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
		Name:         buildGateStageName,
		Kind:         runner.StageKindBuildGate,
		Lane:         "go-native",
		Dependencies: []string{mods.StageNameHuman},
		CacheKey:     "go-native/cache@manifest=2025-09-26@aster=plan",
		Constraints:  runner.StageConstraints{Manifest: manifest},
		Aster: runner.StageAster{
			Enabled: true,
			Toggles: []string{"plan"},
			Bundles: []aster.Metadata{{BundleID: "bundle-1", Stage: buildGateStageName, Toggle: "plan", ArtifactCID: "bafy123", Digest: "sha256:abc"}},
		},
		Job: runner.StageJobSpec{
			Image:   "registry.dev/build:latest",
			Command: []string{"/bin/build"},
			Env: map[string]string{
				"GOFLAGS": "-mod=vendor",
			},
			Resources: runner.StageJobResources{
				CPU:    "4000m",
				Memory: "8Gi",
			},
		},
	}

	fake := &fakeWorkflowRPC{
		resp: workflowrpc.SubmitResponse{
			Status:    string(runner.StageStatusCompleted),
			Message:   "ok",
			Retryable: false,
			Artifacts: []workflowrpc.Artifact{{
				Name:        "mods-plan",
				ArtifactCID: "cid-mods-plan",
				Digest:      "sha256:modsplan",
				MediaType:   "application/tar+zst",
			}},
		},
	}

	client, err := NewClient(Options{
		Endpoint:    "https://grid.dev",
		BearerToken: "token-123",
		Retries:     5,
		HelperFactory: func(opts rpcHelper.Options) (workflowRPCClient, error) {
			if got, want := opts.Endpoint, "https://grid.dev"; got != want {
				t.Fatalf("unexpected endpoint: got %s, want %s", got, want)
			}
			if got, want := opts.BearerToken, "token-123"; got != want {
				t.Fatalf("unexpected bearer token: got %q, want %q", got, want)
			}
			if got, want := opts.Retries, 5; got != want {
				t.Fatalf("unexpected retries: got %d, want %d", got, want)
			}
			return fake, nil
		},
	})
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
	if len(outcome.Artifacts) != 1 {
		t.Fatalf("expected artifact manifest, got %#v", outcome.Artifacts)
	}
	artifact := outcome.Artifacts[0]
	if artifact.ArtifactCID != "cid-mods-plan" || artifact.Digest != "sha256:modsplan" {
		t.Fatalf("unexpected artifact manifest: %#v", artifact)
	}

	if len(fake.requests) != 1 {
		t.Fatalf("expected 1 submit request, got %d", len(fake.requests))
	}
	req := fake.requests[0]
	if req.SchemaVersion != contracts.SchemaVersion {
		t.Fatalf("unexpected schema version: %s", req.SchemaVersion)
	}
	if req.Ticket.TicketID != ticket.TicketID {
		t.Fatalf("ticket mismatch: %s", req.Ticket.TicketID)
	}
	if req.Stage.Name != stage.Name {
		t.Fatalf("stage name mismatch: %s", req.Stage.Name)
	}
	if req.Stage.Kind != string(stage.Kind) {
		t.Fatalf("stage kind mismatch: %s", req.Stage.Kind)
	}
	if req.Stage.Lane != stage.Lane {
		t.Fatalf("lane mismatch: %s", req.Stage.Lane)
	}
	if req.Stage.CacheKey != stage.CacheKey {
		t.Fatalf("cache key mismatch: %s", req.Stage.CacheKey)
	}
	if req.Stage.Aster.Enabled != stage.Aster.Enabled {
		t.Fatalf("aster enabled mismatch: %v", req.Stage.Aster.Enabled)
	}
	if len(req.Stage.Aster.Bundles) != len(stage.Aster.Bundles) {
		t.Fatalf("expected %d bundles, got %d", len(stage.Aster.Bundles), len(req.Stage.Aster.Bundles))
	}
	if req.Stage.Constraints.Manifest.Manifest.Name != manifest.Manifest.Name {
		t.Fatalf("manifest name mismatch: %s", req.Stage.Constraints.Manifest.Manifest.Name)
	}
	if req.Stage.Job.Image != stage.Job.Image {
		t.Fatalf("job image mismatch: %s", req.Stage.Job.Image)
	}
	if !reflect.DeepEqual(req.Stage.Job.Command, stage.Job.Command) {
		t.Fatalf("job command mismatch: %#v", req.Stage.Job.Command)
	}
	if got := req.Stage.Job.Env["GOFLAGS"]; got != "-mod=vendor" {
		t.Fatalf("job env mismatch: %s", got)
	}
	if req.Stage.Job.Resources.CPU != stage.Job.Resources.CPU {
		t.Fatalf("job cpu mismatch: %s", req.Stage.Job.Resources.CPU)
	}
	if req.Stage.Job.Resources.Memory != stage.Job.Resources.Memory {
		t.Fatalf("job memory mismatch: %s", req.Stage.Job.Resources.Memory)
	}
	if req.Stage.Job.Metadata["lane"] != stage.Lane {
		t.Fatalf("job metadata missing lane: %#v", req.Stage.Job.Metadata)
	}
	if req.Stage.Job.Metadata["cache_key"] != stage.CacheKey {
		t.Fatalf("job metadata missing cache key: %#v", req.Stage.Job.Metadata)
	}
	if req.Stage.Job.Metadata["priority"] != "standard" {
		t.Fatalf("job metadata missing priority: %#v", req.Stage.Job.Metadata)
	}
	if req.Stage.Job.Metadata["manifest_name"] != manifest.Manifest.Name {
		t.Fatalf("job metadata missing manifest name: %#v", req.Stage.Job.Metadata)
	}
	if req.Stage.Job.Metadata["manifest_version"] != manifest.Manifest.Version {
		t.Fatalf("job metadata missing manifest version: %#v", req.Stage.Job.Metadata)
	}

	invocations := client.Invocations()
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

func TestClientExecuteStagePropagatesRPCError(t *testing.T) {
	fake := &fakeWorkflowRPC{err: errors.New("submit failed")}
	client, err := NewClient(Options{
		Endpoint: "https://grid.dev",
		HelperFactory: func(opts rpcHelper.Options) (workflowRPCClient, error) {
			return fake, nil
		},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.ExecuteStage(context.Background(), contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-1", Tenant: "acme", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}, runner.Stage{Name: mods.StageNamePlan, Kind: runner.StageKindModsPlan, Lane: "node-wasm", Constraints: runner.StageConstraints{Manifest: manifests.Compilation{Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"}}}}, "/tmp")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, fake.err) {
		t.Fatalf("expected submit error to propagate, got %v", err)
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

func TestNewClientPropagatesFactoryError(t *testing.T) {
	boom := errors.New("factory boom")
	_, err := NewClient(Options{
		Endpoint: "https://grid.dev",
		HelperFactory: func(opts rpcHelper.Options) (workflowRPCClient, error) {
			return nil, boom
		},
	})
	if err == nil {
		t.Fatal("expected error when factory fails")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("unexpected error: %v", err)
	}
}

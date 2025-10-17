package grid

import (
	"context"
	"errors"
	"reflect"
	"testing"

	workflowsdk "github.com/iw2rmb/grid/sdk/workflowrpc/go"
	helper "github.com/iw2rmb/grid/sdk/workflowrpc/helper"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

const buildGateStageName = "build-gate"

func TestClientExecuteStageSuccess(t *testing.T) {
	ticket := contracts.WorkflowTicket{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      "ticket-123",
		Tenant:        "acme",
		Manifest:      contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"},
	}

	manifest := manifests.Compilation{
		Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26", Summary: "sample"},
		ManifestVersion: "v2",
		Lanes:           manifests.LaneSet{Required: []manifests.Lane{{Name: "build-gate"}}},
		Aster:           manifests.AsterSet{Required: []string{"plan"}},
	}

	stage := runner.Stage{
		Name:         buildGateStageName,
		Kind:         runner.StageKindBuildGate,
		Lane:         "build-gate",
		Dependencies: []string{mods.StageNameHuman},
		CacheKey:     "build-gate/build-gate@manifest=2025-09-26@aster=plan",
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
			Runtime: "docker",
		},
	}

	fake := newFakeWorkflowClient(t)
	fake.submitResp = workflowsdk.SubmitResponse{
		RunID:         "run-123",
		WorkflowID:    "smoke",
		CorrelationID: "ticket-123",
		Accepted:      true,
		Status:        workflowsdk.RunStatusQueued,
	}

	stream := &fakeStreamer{
		events: []workflowsdk.StatusEvent{
			{RunID: "run-123", Status: workflowsdk.RunStatusDispatch},
			{RunID: "run-123", Status: workflowsdk.RunStatusRunning},
			{RunID: "run-123", Status: workflowsdk.RunStatusSucceeded, Message: "ok"},
		},
	}

	client, err := NewClient(Options{
		Endpoint: "https://grid.dev",
		HelperFactory: func(cfg helper.Config) (workflowClient, error) {
			return fake, nil
		},
		StreamFunc: stream.Stream,
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
	if len(outcome.Artifacts) != 0 {
		t.Fatalf("expected no artifacts, got %#v", outcome.Artifacts)
	}

	if len(fake.submits) != 1 {
		t.Fatalf("expected 1 submit request, got %d", len(fake.submits))
	}
	req := fake.submits[0]
	if req.Tenant != "acme" {
		t.Fatalf("tenant mismatch: %s", req.Tenant)
	}
	if req.WorkflowID != "smoke" {
		t.Fatalf("workflow id mismatch: %s", req.WorkflowID)
	}
	if req.RunMetadata.CorrelationID != "ticket-123" {
		t.Fatalf("unexpected correlation id: %s", req.RunMetadata.CorrelationID)
	}
	if req.Job.Image != stage.Job.Image {
		t.Fatalf("job image mismatch: %s", req.Job.Image)
	}
	if !reflect.DeepEqual(req.Job.Command, stage.Job.Command) {
		t.Fatalf("job command mismatch: %#v", req.Job.Command)
	}
	if got := req.Job.Env["GOFLAGS"]; got != "-mod=vendor" {
		t.Fatalf("job env mismatch: %s", got)
	}
	if lane, ok := req.Job.Metadata["lane"]; !ok || lane != stage.Lane {
		t.Fatalf("expected lane metadata, got %#v", req.Job.Metadata)
	}
	if req.Job.Runtime != stage.Job.Runtime {
		t.Fatalf("expected runtime %s, got %s", stage.Job.Runtime, req.Job.Runtime)
	}

	if stream.calls != 1 {
		t.Fatalf("expected stream to be invoked once, got %d", stream.calls)
	}
	if stream.lastReq.RunID != "run-123" {
		t.Fatalf("unexpected stream run id: %s", stream.lastReq.RunID)
	}

	invocations := client.Invocations()
	if len(invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(invocations))
	}
	if invocations[0].TicketID != ticket.TicketID {
		t.Fatalf("invocation ticket mismatch: %s", invocations[0].TicketID)
	}
}

func TestClientExecuteStagePropagatesSubmitError(t *testing.T) {
	fake := newFakeWorkflowClient(t)
	fake.submitErr = errors.New("submit failed")

	client, err := NewClient(Options{
		Endpoint: "https://grid.dev",
		HelperFactory: func(cfg helper.Config) (workflowClient, error) {
			return fake, nil
		},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.ExecuteStage(
		context.Background(),
		contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-1", Tenant: "acme", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}},
		runner.Stage{
			Name: mods.StageNamePlan,
			Kind: runner.StageKindModsPlan,
			Job:  runner.StageJobSpec{Image: "alpine"},
		},
		"/tmp",
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, fake.submitErr) {
		t.Fatalf("expected submit error to propagate, got %v", err)
	}
}

func TestNewClientValidatesEndpoint(t *testing.T) {
	if _, err := NewClient(Options{}); err == nil {
		t.Fatal("expected error for empty endpoint")
	}
	if _, err := NewClient(Options{Endpoint: "://invalid"}); err == nil {
		t.Fatal("expected error for invalid endpoint")
	}
}

func TestNewClientPropagatesFactoryError(t *testing.T) {
	boom := errors.New("factory boom")
	_, err := NewClient(Options{
		Endpoint: "https://grid.dev",
		HelperFactory: func(helper.Config) (workflowClient, error) {
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

type fakeWorkflowClient struct {
	client       *workflowsdk.Client
	submits      []workflowsdk.SubmitRequest
	submitResp   workflowsdk.SubmitResponse
	submitErr    error
	metadataReqs []workflowsdk.MetadataRequest
	metadataResp workflowsdk.MetadataResponse
	metadataErr  error
	cancelReqs   []workflowsdk.CancelRequest
	cancelResp   workflowsdk.CancelResponse
	cancelErr    error
}

func newFakeWorkflowClient(t *testing.T) *fakeWorkflowClient {
	t.Helper()
	client, err := workflowsdk.NewClient("https://grid.dev")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return &fakeWorkflowClient{client: client}
}

func (f *fakeWorkflowClient) Submit(ctx context.Context, req workflowsdk.SubmitRequest) (workflowsdk.SubmitResponse, error) {
	_ = ctx
	f.submits = append(f.submits, req)
	if f.submitErr != nil {
		return workflowsdk.SubmitResponse{}, f.submitErr
	}
	return f.submitResp, nil
}

func (f *fakeWorkflowClient) Metadata(ctx context.Context, req workflowsdk.MetadataRequest) (workflowsdk.MetadataResponse, error) {
	_ = ctx
	f.metadataReqs = append(f.metadataReqs, req)
	if f.metadataErr != nil {
		return workflowsdk.MetadataResponse{}, f.metadataErr
	}
	return f.metadataResp, nil
}

func (f *fakeWorkflowClient) Cancel(ctx context.Context, req workflowsdk.CancelRequest) (workflowsdk.CancelResponse, error) {
	_ = ctx
	f.cancelReqs = append(f.cancelReqs, req)
	if f.cancelErr != nil {
		return workflowsdk.CancelResponse{}, f.cancelErr
	}
	return f.cancelResp, nil
}

func (f *fakeWorkflowClient) Client() *workflowsdk.Client {
	return f.client
}

type fakeStreamer struct {
	calls   int
	lastReq workflowsdk.StreamRequest
	events  []workflowsdk.StatusEvent
}

func (s *fakeStreamer) Stream(ctx context.Context, _ *workflowsdk.Client, req workflowsdk.StreamRequest, handler func(workflowsdk.StatusEvent) error, opts helper.StreamOptions) error {
	_ = opts
	s.calls++
	s.lastReq = req
	for _, evt := range s.events {
		if err := handler(evt); err != nil {
			return err
		}
	}
	return context.Canceled
}

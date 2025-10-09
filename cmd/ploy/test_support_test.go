package main

import (
	"context"
	"strings"
	"testing"

	gridclient "github.com/iw2rmb/grid/sdk/gridclient/go"
	workflowsdk "github.com/iw2rmb/grid/sdk/workflowrpc/go"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

type recordingRunner struct {
	opts runner.Options
	err  error
}

func (r *recordingRunner) Run(ctx context.Context, opts runner.Options) error {
	r.opts = opts
	return r.err
}

func defaultManifestPayload() manifests.Compilation {
	return manifests.Compilation{
		Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
		ManifestVersion: "v2",
		Lanes:           manifests.LaneSet{Required: []manifests.Lane{{Name: "mods-plan"}, {Name: "mods-java"}, {Name: "mods-llm"}, {Name: "mods-human"}, {Name: "go-native"}}},
	}
}

type stubManifestCompiler struct {
	compiled manifests.Compilation
	err      error
	ref      contracts.ManifestReference
}

func (s *stubManifestCompiler) Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error) {
	_ = ctx
	s.ref = ref
	return s.compiled, s.err
}

type recordingLocator struct {
	dir string
}

func (r *recordingLocator) Locate(ctx context.Context, req aster.Request) (aster.Metadata, error) {
	_ = ctx
	return aster.Metadata{Stage: req.Stage, Toggle: req.Toggle}, nil
}

type recordingEnvironmentService struct {
	request environments.Request
	result  environments.Result
	err     error
}

func (r *recordingEnvironmentService) Materialize(ctx context.Context, req environments.Request) (environments.Result, error) {
	r.request = req
	return r.result, r.err
}

type fakeLaneRegistry struct {
	description lanes.Description
	err         error
}

func (f *fakeLaneRegistry) Describe(name string, opts lanes.DescribeOptions) (lanes.Description, error) {
	if f.err != nil {
		return lanes.Description{}, f.err
	}
	f.description.Parameters = opts
	if strings.TrimSpace(f.description.Lane.Job.Image) == "" {
		if f.description.Lane.Job.Env == nil {
			f.description.Lane.Job.Env = map[string]string{}
		}
		if len(f.description.Lane.Job.Command) == 0 {
			f.description.Lane.Job.Command = []string{"/bin/true"}
		}
		if f.description.Lane.Job.Resources.CPU == "" {
			f.description.Lane.Job.Resources.CPU = "1000m"
		}
		if f.description.Lane.Job.Resources.Memory == "" {
			f.description.Lane.Job.Resources.Memory = "1Gi"
		}
		f.description.Lane.Job.Image = "registry.dev/default:latest"
	}
	return f.description, nil
}

type stubWorkspacePreparer struct {
	calls []runner.WorkspacePrepareRequest
	err   error
}

func (s *stubWorkspacePreparer) Prepare(ctx context.Context, req runner.WorkspacePrepareRequest) error {
	s.calls = append(s.calls, req)
	return s.err
}

func withStubWorkspacePreparer(t *testing.T) *stubWorkspacePreparer {
	prev := workspacePreparerFactory
	stub := &stubWorkspacePreparer{}
	workspacePreparerFactory = func() (runner.WorkspacePreparer, error) { return stub, nil }
	if t != nil {
		t.Cleanup(func() { workspacePreparerFactory = prev })
	}
	return stub
}

type stubGridClient struct {
	status        gridclient.Status
	workflow      *workflowsdk.Client
	workflowError error
	calls         int
}

func newStubGridClient(status gridclient.Status) *stubGridClient {
	return &stubGridClient{
		status:   status,
		workflow: &workflowsdk.Client{},
	}
}

func (s *stubGridClient) Status() gridclient.Status {
	return s.status
}

func (s *stubGridClient) WorkflowClient(context.Context) (*workflowsdk.Client, error) {
	s.calls++
	if s.workflowError != nil {
		return nil, s.workflowError
	}
	if s.workflow != nil {
		return s.workflow, nil
	}
	return &workflowsdk.Client{}, nil
}

func withGridClientStub(t *testing.T, stub gridClientAPI) {
	if t != nil {
		t.Helper()
	}

	prevNew := newGridClient
	resetGridClientState()
	newGridClient = func(context.Context, gridclient.Config) (gridClientAPI, error) {
		return stub, nil
	}
	if t != nil {
		t.Cleanup(func() {
			newGridClient = prevNew
			resetGridClientState()
		})
	}
}

package grid

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	workflowsdk "github.com/iw2rmb/grid/sdk/workflowrpc/go"
	helper "github.com/iw2rmb/grid/sdk/workflowrpc/helper"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// Options configures the Grid Workflow RPC client.
type Options struct {
	Endpoint              string
	HTTPClient            *http.Client
	BearerToken           string
	WorkflowResolver      WorkflowResolver
	WorkflowClientFactory func(context.Context) (*workflowsdk.Client, error)
	StreamOptions         helper.StreamOptions
	StreamFunc            streamFunc
	HelperFactory         workflowClientFactory
	CursorStoreFactory    CursorStoreFactory
	ControlPlaneHTTP      func(context.Context) (*http.Client, error)
	ControlPlaneStatus    func() ControlPlaneStatus
	LogTailLines          int
}

// WorkflowResolver resolves the workflow identifier associated with a ticket and stage.
type WorkflowResolver func(contracts.WorkflowTicket, runner.Stage) string

type workflowClientFactory func(helper.Config) (workflowClient, error)

type workflowClient interface {
	Submit(ctx context.Context, req workflowsdk.SubmitRequest) (workflowsdk.SubmitResponse, error)
	Metadata(ctx context.Context, req workflowsdk.MetadataRequest) (workflowsdk.MetadataResponse, error)
	Cancel(ctx context.Context, req workflowsdk.CancelRequest) (workflowsdk.CancelResponse, error)
	Client() *workflowsdk.Client
}

type streamFunc func(context.Context, *workflowsdk.Client, workflowsdk.StreamRequest, func(workflowsdk.StatusEvent) error, helper.StreamOptions) error

type CursorStoreFactory func(tenant, workflowID, runID string) (helper.CursorStore, error)

// ControlPlaneStatus summarises the Grid control-plane endpoints exposed by the grid client.
type ControlPlaneStatus struct {
	APIEndpoint string
}

// Client implements runner.GridClient by dispatching workflow stages to Grid's Workflow RPC.
type Client struct {
	rpc             workflowClient
	stream          streamFunc
	streamOpts      helper.StreamOptions
	resolveWorkflow WorkflowResolver
	cursorFactory   CursorStoreFactory
	controlHTTP     func(context.Context) (*http.Client, error)
	controlStatus   func() ControlPlaneStatus
	logTail         int

	mu          sync.Mutex
	invocations []runner.StageInvocation
}

// NewClient constructs a Grid Workflow RPC client.
func NewClient(opts Options) (*Client, error) {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		if opts.WorkflowClientFactory == nil {
			return nil, fmt.Errorf("grid endpoint is required")
		}
	} else {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("parse grid endpoint: %w", err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("grid endpoint must include scheme and host")
		}
	}

	factory := opts.HelperFactory
	switch {
	case opts.WorkflowClientFactory != nil:
		factory = func(helper.Config) (workflowClient, error) {
			client, err := opts.WorkflowClientFactory(context.Background())
			if err != nil {
				return nil, err
			}
			if client == nil {
				return nil, fmt.Errorf("workflow client factory returned nil client")
			}
			return &helperWorkflowClient{client: client}, nil
		}
	case factory == nil:
		factory = defaultWorkflowClientFactory
	}

	cfgOpts := []helper.ConfigOption{}
	if endpoint != "" {
		cfgOpts = append(cfgOpts, helper.WithEndpoint(endpoint))
	}
	if opts.HTTPClient != nil {
		cfgOpts = append(cfgOpts, helper.WithHTTPClient(opts.HTTPClient))
	}
	if token := strings.TrimSpace(opts.BearerToken); token != "" {
		cfgOpts = append(cfgOpts, helper.WithTokenProvider(helper.StaticTokenProvider(token)))
	}

	cfg := helper.NewConfig(cfgOpts...)
	rpcClient, err := factory(cfg)
	if err != nil {
		return nil, err
	}

	streamFn := opts.StreamFunc
	if streamFn == nil {
		streamFn = helper.StreamStatusWithRetry
	}

	resolver := opts.WorkflowResolver
	if resolver == nil {
		resolver = defaultWorkflowResolver
	}

	cursorFactory := opts.CursorStoreFactory
	if cursorFactory == nil {
		cursorFactory = func(string, string, string) (helper.CursorStore, error) {
			return &helper.MemoryCursorStore{}, nil
		}
	}

	logTail := opts.LogTailLines
	if logTail <= 0 {
		logTail = 200
	}

	return &Client{
		rpc:             rpcClient,
		stream:          streamFn,
		streamOpts:      opts.StreamOptions,
		resolveWorkflow: resolver,
		cursorFactory:   cursorFactory,
		controlHTTP:     opts.ControlPlaneHTTP,
		controlStatus:   opts.ControlPlaneStatus,
		logTail:         logTail,
	}, nil
}

// ExecuteStage submits the stage execution request to Grid and returns the resulting outcome.
func (c *Client) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	if err := ctx.Err(); err != nil {
		return runner.StageOutcome{}, err
	}

	workflowID := strings.TrimSpace(c.resolveWorkflow(ticket, stage))
	if workflowID == "" {
		workflowID = strings.TrimSpace(ticket.TicketID)
	}

	submitReq, err := buildSubmitRequest(ticket, stage, workflowID)
	if err != nil {
		return runner.StageOutcome{}, err
	}

	resp, err := c.rpc.Submit(ctx, submitReq)
	if err != nil {
		return runner.StageOutcome{}, err
	}
	runID := strings.TrimSpace(resp.RunID)

	term, err := c.awaitTerminalStatus(ctx, runID, ticket.Tenant, workflowID)
	if err != nil {
		return runner.StageOutcome{}, err
	}
	evidence := c.collectEvidence(ctx, runID, term)

	outcome := runner.StageOutcome{
		Stage:    stage,
		RunID:    runID,
		Status:   mapRunStatus(term.status),
		Message:  term.message,
		Archive:  stageArchiveFromTerminal(term),
		Evidence: evidence,
	}
	if outcome.Status == runner.StageStatusFailed {
		outcome.Retryable = false
	}
	c.recordInvocation(ticket, outcome.Stage, workspace, runID, outcome.Archive, evidence, outcome.Artifacts)
	return outcome, nil
}

// CancelWorkflow requests cancellation for the specified workflow run.
func (c *Client) CancelWorkflow(ctx context.Context, req runner.CancelRequest) (runner.CancelResult, error) {
	trimmedRunID := strings.TrimSpace(req.RunID)
	if trimmedRunID == "" {
		return runner.CancelResult{}, fmt.Errorf("workflow run id is required")
	}
	trimmedTenant := strings.TrimSpace(req.Tenant)
	if trimmedTenant == "" {
		return runner.CancelResult{}, fmt.Errorf("tenant is required")
	}
	resp, err := c.rpc.Cancel(ctx, workflowsdk.CancelRequest{
		Tenant:     trimmedTenant,
		WorkflowID: strings.TrimSpace(req.WorkflowID),
		RunID:      trimmedRunID,
		Reason:     strings.TrimSpace(req.Reason),
	})
	if err != nil {
		return runner.CancelResult{}, err
	}
	return runner.CancelResult{
		RunID:     strings.TrimSpace(resp.RunID),
		Status:    mapRunStatus(resp.Status),
		Requested: resp.Canceled,
	}, nil
}

// Invocations returns a snapshot of recorded stage invocations for CLI summaries.
func (c *Client) Invocations() []runner.StageInvocation {
	c.mu.Lock()
	defer c.mu.Unlock()
	dup := make([]runner.StageInvocation, len(c.invocations))
	copy(dup, c.invocations)
	return dup
}

func (c *Client) recordInvocation(ticket contracts.WorkflowTicket, stage runner.Stage, workspace, runID string, archive *runner.StageArchive, evidence *runner.StageEvidence, artifacts []runner.Artifact) {
	c.mu.Lock()
	defer c.mu.Unlock()
	invocation := runner.StageInvocation{TicketID: ticket.TicketID, Stage: stage, Workspace: workspace, RunID: runID, Archive: archive, Evidence: evidence}
	if len(artifacts) > 0 {
		invocation.Artifacts = cloneArtifacts(artifacts)
	}
	c.invocations = append(c.invocations, invocation)
}

func cloneArtifacts(src []runner.Artifact) []runner.Artifact {
	if len(src) == 0 {
		return nil
	}
	dst := make([]runner.Artifact, len(src))
	copy(dst, src)
	return dst
}

func defaultWorkflowClientFactory(cfg helper.Config) (workflowClient, error) {
	client, err := cfg.Client(context.Background())
	if err != nil {
		return nil, err
	}
	return &helperWorkflowClient{client: client}, nil
}

func defaultWorkflowResolver(ticket contracts.WorkflowTicket, stage runner.Stage) string {
	if name := strings.TrimSpace(ticket.Manifest.Name); name != "" {
		return name
	}
	return strings.TrimSpace(ticket.TicketID)
}

type helperWorkflowClient struct {
	client *workflowsdk.Client
}

func (h *helperWorkflowClient) Submit(ctx context.Context, req workflowsdk.SubmitRequest) (workflowsdk.SubmitResponse, error) {
	return h.client.Submit(ctx, req)
}

func (h *helperWorkflowClient) Metadata(ctx context.Context, req workflowsdk.MetadataRequest) (workflowsdk.MetadataResponse, error) {
	return h.client.Metadata(ctx, req)
}

func (h *helperWorkflowClient) Cancel(ctx context.Context, req workflowsdk.CancelRequest) (workflowsdk.CancelResponse, error) {
	return h.client.Cancel(ctx, req)
}

func (h *helperWorkflowClient) Client() *workflowsdk.Client {
	return h.client
}

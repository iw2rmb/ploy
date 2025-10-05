package grid

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	workflowsdk "github.com/iw2rmb/grid/sdk/workflowrpc/go"
	helper "github.com/iw2rmb/grid/sdk/workflowrpc/helper"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// Options configures the Grid Workflow RPC client.
type Options struct {
	Endpoint           string
	HTTPClient         *http.Client
	BearerToken        string
	WorkflowResolver   WorkflowResolver
	StreamOptions      helper.StreamOptions
	StreamFunc         streamFunc
	HelperFactory      workflowClientFactory
	CursorStoreFactory CursorStoreFactory
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

type terminalRun struct {
	status   workflowsdk.RunStatus
	message  string
	metadata map[string]string
	result   map[string]any
}

const (
	archiveResultIDKey       = "archive_export_id"
	archiveResultClassKey    = "archive_export_class"
	archiveResultQueuedAtKey = "archive_export_queued_at"
)

// Client implements runner.GridClient by dispatching workflow stages to Grid's Workflow RPC.
type Client struct {
	rpc             workflowClient
	stream          streamFunc
	streamOpts      helper.StreamOptions
	resolveWorkflow WorkflowResolver
	cursorFactory   CursorStoreFactory

	mu          sync.Mutex
	invocations []runner.StageInvocation
}

// NewClient constructs a Grid Workflow RPC client.
func NewClient(opts Options) (*Client, error) {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("grid endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse grid endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("grid endpoint must include scheme and host")
	}

	factory := opts.HelperFactory
	if factory == nil {
		factory = defaultWorkflowClientFactory
	}

	cfgOpts := []helper.ConfigOption{helper.WithEndpoint(endpoint)}
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

	return &Client{
		rpc:             rpcClient,
		stream:          streamFn,
		streamOpts:      opts.StreamOptions,
		resolveWorkflow: resolver,
		cursorFactory:   cursorFactory,
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

	outcome := runner.StageOutcome{
		Stage:   stage,
		RunID:   runID,
		Status:  mapRunStatus(term.status),
		Message: term.message,
		Archive: stageArchiveFromTerminal(term),
	}
	if outcome.Status == runner.StageStatusFailed {
		outcome.Retryable = false
	}
	c.recordInvocation(ticket, outcome.Stage, workspace, runID, outcome.Archive)
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

func (c *Client) recordInvocation(ticket contracts.WorkflowTicket, stage runner.Stage, workspace, runID string, archive *runner.StageArchive) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invocations = append(c.invocations, runner.StageInvocation{TicketID: ticket.TicketID, Stage: stage, Workspace: workspace, RunID: runID, Archive: archive})
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

func (c *Client) awaitTerminalStatus(ctx context.Context, runID, tenant, workflowID string) (terminalRun, error) {
	streamReq := workflowsdk.StreamRequest{
		Tenant:     strings.TrimSpace(tenant),
		WorkflowID: strings.TrimSpace(workflowID),
		RunID:      strings.TrimSpace(runID),
	}
	if streamReq.RunID == "" {
		return terminalRun{}, fmt.Errorf("workflow run id is required")
	}

	var final terminalRun

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	opts := c.streamOpts
	if opts.CursorStore == nil && c.cursorFactory != nil {
		store, err := c.cursorFactory(streamReq.Tenant, streamReq.WorkflowID, streamReq.RunID)
		if err != nil {
			return terminalRun{}, err
		}
		opts.CursorStore = store
	}

	streamErr := c.stream(streamCtx, c.rpc.Client(), streamReq, func(evt workflowsdk.StatusEvent) error {
		if evt.RunID == "" {
			return nil
		}
		if message := strings.TrimSpace(evt.Message); message != "" {
			final.message = message
		}
		if isTerminalStatus(evt.Status) {
			final.status = evt.Status
			cancel()
		}
		return nil
	}, opts)

	if streamErr != nil {
		if ctx.Err() != nil {
			return terminalRun{}, ctx.Err()
		}
		if errors.Is(streamErr, context.Canceled) && final.status != "" {
			streamErr = nil
		}
	}

	meta, err := c.rpc.Metadata(ctx, workflowsdk.MetadataRequest{Tenant: tenant, WorkflowID: workflowID, RunID: runID})
	if err != nil {
		if final.status == "" {
			if streamErr != nil {
				return terminalRun{}, streamErr
			}
			return terminalRun{}, err
		}
	} else {
		final.metadata = copyStringMap(meta.Run.Metadata)
		final.result = cloneAnyMap(meta.Run.Result)
		if final.status == "" {
			final.status = meta.Run.Status
		}
		if final.message == "" && meta.Run.Result != nil {
			if msg, ok := meta.Run.Result["reason"].(string); ok {
				final.message = strings.TrimSpace(msg)
			}
		}
	}

	if final.status == "" {
		if streamErr != nil {
			return terminalRun{}, streamErr
		}
		final.status = workflowsdk.RunStatusSucceeded
	}

	return final, nil
}

func buildSubmitRequest(ticket contracts.WorkflowTicket, stage runner.Stage, workflowID string) (workflowsdk.SubmitRequest, error) {
	builder := helper.NewSubmitBuilder().
		Tenant(strings.TrimSpace(ticket.Tenant)).
		Workflow(strings.TrimSpace(workflowID)).
		Correlation(strings.TrimSpace(ticket.TicketID)).
		Idempotency(idempotencyKey(ticket, stage)).
		Label("stage", stage.Name).
		Label("lane", stage.Lane).
		Label("ticket_id", ticket.TicketID)

	manifest := stage.Constraints.Manifest.Manifest
	if manifest.Name != "" {
		builder.Label("manifest_name", manifest.Name)
	}
	if manifest.Version != "" {
		builder.Label("manifest_version", manifest.Version)
	}

	builder.Job(func(job *helper.JobBuilder) {
		if strings.TrimSpace(stage.Job.Image) != "" {
			job.Image(stage.Job.Image)
		}
		if len(stage.Job.Command) > 0 {
			job.Command(stage.Job.Command...)
		}
		for key, value := range stage.Job.Env {
			job.Env(key, value)
		}
		for key, value := range copyStringMap(stage.Job.Metadata) {
			job.Metadata(key, value)
		}
		if _, ok := stage.Job.Metadata["priority"]; !ok || strings.TrimSpace(stage.Job.Metadata["priority"]) == "" {
			job.Metadata("priority", "standard")
		}
		if stage.Lane != "" {
			job.Metadata("lane", stage.Lane)
		}
		if stage.CacheKey != "" {
			job.Metadata("cache_key", stage.CacheKey)
		}
		if manifest.Name != "" {
			job.Metadata("manifest_name", manifest.Name)
		}
		if manifest.Version != "" {
			job.Metadata("manifest_version", manifest.Version)
		}
		if stage.Job.Resources.CPU != "" {
			job.Metadata("resources.cpu", stage.Job.Resources.CPU)
		}
		if stage.Job.Resources.Memory != "" {
			job.Metadata("resources.memory", stage.Job.Resources.Memory)
		}
		if stage.Job.Resources.Disk != "" {
			job.Metadata("resources.disk", stage.Job.Resources.Disk)
		}
		if stage.Job.Resources.GPU != "" {
			job.Metadata("resources.gpu", stage.Job.Resources.GPU)
		}
		if len(stage.Aster.Toggles) > 0 {
			job.Metadata("aster.toggles", append([]string(nil), stage.Aster.Toggles...))
		}
		if len(stage.Aster.Bundles) > 0 {
			job.Metadata("aster.bundles", append([]aster.Metadata(nil), stage.Aster.Bundles...))
		}
	})

	return builder.Build()
}

func idempotencyKey(ticket contracts.WorkflowTicket, stage runner.Stage) string {
	base := strings.TrimSpace(ticket.TicketID)
	if base == "" {
		base = strings.TrimSpace(ticket.Manifest.Name)
	}
	stageName := strings.TrimSpace(stage.Name)
	if stageName == "" {
		stageName = "stage"
	}
	return fmt.Sprintf("%s:%s", base, stageName)
}

func mapRunStatus(status workflowsdk.RunStatus) runner.StageStatus {
	switch status {
	case workflowsdk.RunStatusSucceeded:
		return runner.StageStatusCompleted
	case workflowsdk.RunStatusFailed, workflowsdk.RunStatusCanceled:
		return runner.StageStatusFailed
	default:
		return runner.StageStatusRunning
	}
}

func isTerminalStatus(status workflowsdk.RunStatus) bool {
	switch status {
	case workflowsdk.RunStatusSucceeded, workflowsdk.RunStatusFailed, workflowsdk.RunStatusCanceled:
		return true
	default:
		return false
	}
}

func stageArchiveFromTerminal(term terminalRun) *runner.StageArchive {
	if len(term.result) == 0 {
		return nil
	}
	id := strings.TrimSpace(stringFromAny(term.result[archiveResultIDKey]))
	class := strings.TrimSpace(stringFromAny(term.result[archiveResultClassKey]))
	queued := strings.TrimSpace(stringFromAny(term.result[archiveResultQueuedAtKey]))
	if id == "" && class == "" && queued == "" {
		return nil
	}
	var queuedAt time.Time
	if queued != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, queued); err == nil {
			queuedAt = parsed
		}
	}
	return &runner.StageArchive{ID: id, Class: class, QueuedAt: queuedAt}
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case fmt.Stringer:
		return v.String()
	case []byte:
		return string(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%f", v)
	default:
		return ""
	}
}

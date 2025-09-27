package grid

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/grid/workflowrpc"
	rpcHelper "github.com/iw2rmb/ploy/internal/workflow/grid/workflowrpc/helper"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// Options configures the Grid Workflow RPC client.
type Options struct {
	Endpoint      string
	HTTPClient    *http.Client
	BearerToken   string
	Retries       int
	HelperFactory workflowRPCHelperFactory
}

type workflowRPCHelperFactory func(rpcHelper.Options) (workflowRPCClient, error)

type workflowRPCClient interface {
	Submit(ctx context.Context, req workflowrpc.SubmitRequest) (workflowrpc.SubmitResponse, error)
}

// Client implements runner.GridClient by dispatching workflow stages to Grid's Workflow RPC.
type Client struct {
	rpc workflowRPCClient

	mu          sync.Mutex
	invocations []runner.StageInvocation
}

const defaultJobPriority = "standard"

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
		factory = func(helperOpts rpcHelper.Options) (workflowRPCClient, error) {
			return rpcHelper.New(helperOpts)
		}
	}

	rpcClient, err := factory(rpcHelper.Options{Endpoint: endpoint, HTTPClient: opts.HTTPClient, BearerToken: opts.BearerToken, Retries: opts.Retries})
	if err != nil {
		return nil, err
	}

	return &Client{rpc: rpcClient}, nil
}

// ExecuteStage submits the stage execution request to Grid and returns the resulting outcome.
func (c *Client) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	c.recordInvocation(ticket, stage, workspace)

	req := workflowrpc.SubmitRequest{
		SchemaVersion: contracts.SchemaVersion,
		Ticket:        ticket,
		Stage:         marshalStage(stage),
	}

	resp, err := c.rpc.Submit(ctx, req)
	if err != nil {
		return runner.StageOutcome{}, err
	}

	outcome := runner.StageOutcome{
		Stage:     stage,
		Status:    runner.StageStatus(resp.Status),
		Retryable: resp.Retryable,
		Message:   resp.Message,
	}
	if outcome.Status == "" {
		outcome.Status = runner.StageStatusCompleted
	}
	if resp.Stage.Name != "" {
		outcome.Stage = unmarshalStage(resp.Stage)
	}
	if len(resp.Artifacts) > 0 {
		artifacts := make([]runner.Artifact, 0, len(resp.Artifacts))
		for _, art := range resp.Artifacts {
			artifacts = append(artifacts, runner.Artifact{
				Name:        strings.TrimSpace(art.Name),
				ArtifactCID: strings.TrimSpace(art.ArtifactCID),
				Digest:      strings.TrimSpace(art.Digest),
				MediaType:   strings.TrimSpace(art.MediaType),
			})
		}
		outcome.Artifacts = artifacts
	}
	return outcome, nil
}

// Invocations returns a snapshot of recorded stage invocations for CLI summaries.
func (c *Client) Invocations() []runner.StageInvocation {
	c.mu.Lock()
	defer c.mu.Unlock()
	dup := make([]runner.StageInvocation, len(c.invocations))
	copy(dup, c.invocations)
	return dup
}

func (c *Client) recordInvocation(ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invocations = append(c.invocations, runner.StageInvocation{TicketID: ticket.TicketID, Stage: stage, Workspace: workspace})
}

func marshalStage(stage runner.Stage) workflowrpc.Stage {
	return workflowrpc.Stage{
		Name:         stage.Name,
		Kind:         string(stage.Kind),
		Lane:         stage.Lane,
		Dependencies: append([]string(nil), stage.Dependencies...),
		CacheKey:     stage.CacheKey,
		Constraints: workflowrpc.Constraints{
			Manifest: stage.Constraints.Manifest,
		},
		Aster: workflowrpc.Aster{
			Enabled: stage.Aster.Enabled,
			Toggles: append([]string(nil), stage.Aster.Toggles...),
			Bundles: append([]aster.Metadata(nil), stage.Aster.Bundles...),
		},
		Job: marshalJob(stage),
	}
}

func unmarshalStage(env workflowrpc.Stage) runner.Stage {
	return runner.Stage{
		Name:         env.Name,
		Kind:         runner.StageKind(env.Kind),
		Lane:         env.Lane,
		Dependencies: append([]string(nil), env.Dependencies...),
		CacheKey:     env.CacheKey,
		Constraints:  runner.StageConstraints{Manifest: env.Constraints.Manifest},
		Aster: runner.StageAster{
			Enabled: env.Aster.Enabled,
			Toggles: append([]string(nil), env.Aster.Toggles...),
			Bundles: append([]aster.Metadata(nil), env.Aster.Bundles...),
		},
		Job: unmarshalJob(env.Job),
	}
}

func marshalJob(stage runner.Stage) workflowrpc.JobSpec {
	metadata := copyStringMap(stage.Job.Metadata)
	if metadata == nil {
		metadata = make(map[string]string)
	}
	if stage.Lane != "" {
		metadata["lane"] = stage.Lane
	}
	if stage.CacheKey != "" {
		metadata["cache_key"] = stage.CacheKey
	}
	if _, ok := metadata["priority"]; !ok || strings.TrimSpace(metadata["priority"]) == "" {
		metadata["priority"] = defaultJobPriority
	}
	manifest := stage.Constraints.Manifest.Manifest
	if manifest.Name != "" {
		metadata["manifest_name"] = manifest.Name
	}
	if manifest.Version != "" {
		metadata["manifest_version"] = manifest.Version
	}

	return workflowrpc.JobSpec{
		Image:   stage.Job.Image,
		Command: append([]string(nil), stage.Job.Command...),
		Env:     copyStringMap(stage.Job.Env),
		Resources: workflowrpc.Resources{
			CPU:    stage.Job.Resources.CPU,
			Memory: stage.Job.Resources.Memory,
			Disk:   stage.Job.Resources.Disk,
			GPU:    stage.Job.Resources.GPU,
		},
		Metadata: metadata,
	}
}

func unmarshalJob(spec workflowrpc.JobSpec) runner.StageJobSpec {
	return runner.StageJobSpec{
		Image:   spec.Image,
		Command: append([]string(nil), spec.Command...),
		Env:     copyStringMap(spec.Env),
		Resources: runner.StageJobResources{
			CPU:    spec.Resources.CPU,
			Memory: spec.Resources.Memory,
			Disk:   spec.Resources.Disk,
			GPU:    spec.Resources.GPU,
		},
		Metadata: copyStringMap(spec.Metadata),
	}
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		if trimmedKey := strings.TrimSpace(key); trimmedKey != "" {
			dst[trimmedKey] = strings.TrimSpace(value)
		}
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

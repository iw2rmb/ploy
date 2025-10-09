//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	gridclient "github.com/iw2rmb/grid/sdk/gridclient/go"
	workflowsdk "github.com/iw2rmb/grid/sdk/workflowrpc/go"
	helper "github.com/iw2rmb/grid/sdk/workflowrpc/helper"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/grid"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// capturingGrid decorates a runner.GridClient to record stage invocations for assertions.
type capturingGrid struct {
	inner       runner.GridClient
	mu          sync.Mutex
	invocations []runner.StageInvocation
}

func newCapturingGrid(inner runner.GridClient) *capturingGrid {
	return &capturingGrid{inner: inner}
}

func (g *capturingGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	g.mu.Lock()
	g.invocations = append(g.invocations, runner.StageInvocation{
		TicketID:  ticket.TicketID,
		Stage:     stage,
		Workspace: workspace,
	})
	idx := len(g.invocations) - 1
	g.mu.Unlock()

	outcome, err := g.inner.ExecuteStage(ctx, ticket, stage, workspace)

	g.mu.Lock()
	if idx >= 0 && idx < len(g.invocations) {
		g.invocations[idx].RunID = outcome.RunID
		g.invocations[idx].Archive = outcome.Archive
	}
	g.mu.Unlock()

	return outcome, err
}

func (g *capturingGrid) CancelWorkflow(ctx context.Context, req runner.CancelRequest) (runner.CancelResult, error) {
	return g.inner.CancelWorkflow(ctx, req)
}

func (g *capturingGrid) Invocations() []runner.StageInvocation {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.invocations) == 0 {
		return nil
	}
	dup := make([]runner.StageInvocation, len(g.invocations))
	copy(dup, g.invocations)
	return dup
}

// liveGridClient constructs a runner.GridClient backed by the real Grid Workflow RPC.
func liveGridClient(cfg Config) (runner.GridClient, string, error) {
	gridID := strings.TrimSpace(cfg.GridID)
	beaconKey := strings.TrimSpace(cfg.BeaconAPIKey)
	if gridID == "" || beaconKey == "" {
		return nil, "", fmt.Errorf("grid id and beacon api key are required")
	}

	stateDir, err := os.MkdirTemp("", "ploy-grid-client-")
	if err != nil {
		return nil, "", fmt.Errorf("create grid client state dir: %w", err)
	}

	clientCfg := gridclient.Config{
		GridID:   gridID,
		APIKey:   beaconKey,
		StateDir: stateDir,
	}
	if beacon := strings.TrimSpace(cfg.BeaconURL); beacon != "" {
		clientCfg.BeaconURL = beacon
	}

	baseClient, err := gridclient.New(context.Background(), clientCfg)
	if err != nil {
		return nil, "", err
	}

	status := baseClient.Status()
	endpoint := strings.TrimSpace(status.Beacon.WorkflowEndpoint)
	if endpoint == "" {
		return nil, "", fmt.Errorf("configure grid client: workflow endpoint unavailable from beacon metadata")
	}

	streamOpts := helper.StreamOptions{HeartbeatInterval: 20 * time.Second, MinBackoff: 200 * time.Millisecond, MaxBackoff: 5 * time.Second}
	cursorFactory := grid.NewCursorStoreFactory(stateDir)
	options := grid.Options{
		Endpoint:           endpoint,
		StreamOptions:      streamOpts,
		CursorStoreFactory: cursorFactory,
		WorkflowClientFactory: func(ctx context.Context) (*workflowsdk.Client, error) {
			return baseClient.WorkflowClient(ctx)
		},
	}

	client, err := grid.NewClient(options)
	if err != nil {
		return nil, "", err
	}
	return client, stateDir, nil
}

package grid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

const stagePath = "/workflow/stages"

// Options configures the Grid Workflow RPC client.
type Options struct {
	Endpoint   string
	HTTPClient *http.Client
}

// Client implements runner.GridClient by dispatching workflow stages to Grid's Workflow RPC.
type Client struct {
	endpoint   *url.URL
	httpClient *http.Client

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

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{endpoint: parsed, httpClient: httpClient}, nil
}

// ExecuteStage submits the stage execution request to Grid and returns the resulting outcome.
func (c *Client) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	c.recordInvocation(ticket, stage, workspace)

	stagePayload := marshalStage(stage)
	payload := executeRequest{
		SchemaVersion: contracts.SchemaVersion,
		Ticket:        ticket,
		Stage:         stagePayload,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return runner.StageOutcome{}, fmt.Errorf("encode grid request: %w", err)
	}

	endpoint := c.endpoint.ResolveReference(&url.URL{Path: stagePath})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(data))
	if err != nil {
		return runner.StageOutcome{}, fmt.Errorf("create grid request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return runner.StageOutcome{}, fmt.Errorf("invoke grid endpoint: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return runner.StageOutcome{}, fmt.Errorf("read grid response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return runner.StageOutcome{}, fmt.Errorf("grid returned status %d: %s", resp.StatusCode, snippet)
	}

	var out executeResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return runner.StageOutcome{}, fmt.Errorf("decode grid response: %w", err)
	}

	outcome := runner.StageOutcome{
		Stage:     stage,
		Status:    runner.StageStatus(out.Status),
		Retryable: out.Retryable,
		Message:   out.Message,
	}
	if outcome.Status == "" {
		outcome.Status = runner.StageStatusCompleted
	}
	if out.Stage.Name != "" {
		outcome.Stage = unmarshalStage(out.Stage)
	}
	if len(out.Artifacts) > 0 {
		artifacts := make([]runner.Artifact, 0, len(out.Artifacts))
		for _, art := range out.Artifacts {
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

type executeRequest struct {
	SchemaVersion string                   `json:"schema_version"`
	Ticket        contracts.WorkflowTicket `json:"ticket"`
	Stage         stageEnvelope            `json:"stage"`
}

type executeResponse struct {
	Status    string             `json:"status"`
	Message   string             `json:"message"`
	Retryable bool               `json:"retryable"`
	Stage     stageEnvelope      `json:"stage"`
	Artifacts []artifactEnvelope `json:"artifacts"`
}

type stageEnvelope struct {
	Name         string              `json:"name"`
	Kind         string              `json:"kind"`
	Lane         string              `json:"lane"`
	Dependencies []string            `json:"dependencies,omitempty"`
	CacheKey     string              `json:"cache_key,omitempty"`
	Constraints  constraintsEnvelope `json:"constraints"`
	Aster        asterEnvelope       `json:"aster"`
}

type constraintsEnvelope struct {
	Manifest manifests.Compilation `json:"manifest"`
}

type asterEnvelope struct {
	Enabled bool             `json:"enabled"`
	Toggles []string         `json:"toggles"`
	Bundles []aster.Metadata `json:"bundles"`
}

func marshalStage(stage runner.Stage) stageEnvelope {
	return stageEnvelope{
		Name:         stage.Name,
		Kind:         string(stage.Kind),
		Lane:         stage.Lane,
		Dependencies: append([]string(nil), stage.Dependencies...),
		CacheKey:     stage.CacheKey,
		Constraints:  constraintsEnvelope{Manifest: stage.Constraints.Manifest},
		Aster: asterEnvelope{
			Enabled: stage.Aster.Enabled,
			Toggles: append([]string(nil), stage.Aster.Toggles...),
			Bundles: append([]aster.Metadata(nil), stage.Aster.Bundles...),
		},
	}
}

func unmarshalStage(env stageEnvelope) runner.Stage {
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
	}
}

type artifactEnvelope struct {
	Name        string `json:"name"`
	ArtifactCID string `json:"artifact_cid"`
	Digest      string `json:"digest"`
	MediaType   string `json:"media_type"`
}

//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
		g.invocations[idx].Evidence = outcome.Evidence
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
	gridKey := strings.TrimSpace(cfg.GridAPIKey)
	if gridID == "" || beaconKey == "" || gridKey == "" {
		return nil, "", fmt.Errorf("grid id, beacon api key, and workflow api key are required")
	}

	stateDir, err := os.MkdirTemp("", "ploy-grid-client-")
	if err != nil {
		return nil, "", fmt.Errorf("create grid client state dir: %w", err)
	}

	httpClient, transport, err := newDualTokenHTTPClient(beaconKey, gridKey)
	if err != nil {
		return nil, "", fmt.Errorf("configure grid http client: %w", err)
	}

	clientCfg := gridclient.Config{
		GridID:     gridID,
		APIKey:     gridKey,
		StateDir:   stateDir,
		HTTPClient: httpClient,
	}
	if beacon := strings.TrimSpace(cfg.BeaconURL); beacon != "" {
		clientCfg.BeaconURL = beacon
	}

	baseClient, err := gridclient.New(context.Background(), clientCfg)
	if err != nil {
		switch {
		case errors.Is(err, gridclient.ErrGridNotFound):
			return nil, "", fmt.Errorf("%w: beacon has no control-plane metadata for grid %q; run `gridctl grid client backfill --grid-id %s` and retry", err, gridID, gridID)
		case errors.Is(err, gridclient.ErrUnauthorized):
			return nil, "", fmt.Errorf("%w: verify GRID_BEACON_API_KEY is scoped to grid %q", err, gridID)
		default:
			return nil, "", err
		}
	}

	status := baseClient.Status()
	endpoint := strings.TrimSpace(status.Beacon.WorkflowEndpoint)
	if endpoint == "" {
		return nil, "", fmt.Errorf("configure grid client: workflow endpoint unavailable from beacon metadata")
	}
	manifestHost := strings.TrimSpace(status.Beacon.ManifestHost)
	if manifestHost == "" {
		return nil, "", fmt.Errorf("configure grid client: manifest host unavailable from beacon metadata")
	}

	caPayload, err := fetchGridCAPayload(endpoint, manifestHost, gridKey)
	if err != nil {
		return nil, "", fmt.Errorf("fetch grid ca: %w", err)
	}
	transport.setCAPayload(caPayload)

	if err := seedManifestCache(stateDir, endpoint, gridID); err != nil {
		return nil, "", fmt.Errorf("seed manifest cache: %w", err)
	}

	streamOpts := helper.StreamOptions{HeartbeatInterval: 20 * time.Second, MinBackoff: 200 * time.Millisecond, MaxBackoff: 5 * time.Second}
	cursorFactory := grid.NewCursorStoreFactory(stateDir)
	options := grid.Options{
		Endpoint:           endpoint,
		HTTPClient:         &http.Client{Transport: transport},
		StreamOptions:      streamOpts,
		CursorStoreFactory: cursorFactory,
		ControlPlaneHTTP: func(ctx context.Context) (*http.Client, error) {
			return baseClient.HTTPClient(ctx)
		},
		ControlPlaneStatus: func() grid.ControlPlaneStatus {
			status := baseClient.Status()
			return grid.ControlPlaneStatus{APIEndpoint: strings.TrimSpace(status.Beacon.APIEndpoint)}
		},
		LogTailLines: 500,
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

func newDualTokenHTTPClient(beaconToken, gridToken string) (*http.Client, *tokenSwitchTransport, error) {
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, nil, fmt.Errorf("gridclient: default transport must be *http.Transport")
	}
	transport := &tokenSwitchTransport{
		base:        baseTransport.Clone(),
		beaconToken: strings.TrimSpace(beaconToken),
		gridToken:   strings.TrimSpace(gridToken),
	}
	return &http.Client{Transport: transport}, transport, nil
}

type tokenSwitchTransport struct {
	base        http.RoundTripper
	beaconToken string
	gridToken   string
	caMu        sync.RWMutex
	caPayload   []byte
}

func (t *tokenSwitchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("gridclient: request is nil")
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()

	if strings.HasPrefix(strings.ToLower(clone.URL.Path), "/v1/tls/ca") {
		if payload := t.getCAPayload(); len(payload) > 0 {
			resp := &http.Response{
				Status:        http.StatusText(http.StatusOK),
				StatusCode:    http.StatusOK,
				Header:        make(http.Header),
				Body:          io.NopCloser(bytes.NewReader(payload)),
				ContentLength: int64(len(payload)),
				Request:       clone,
			}
			resp.Header.Set("Content-Type", "application/json")
			return resp, nil
		}
	}

	if strings.HasPrefix(strings.ToLower(clone.URL.Path), "/v1/grids") && t.beaconToken != "" {
		clone.Header.Set("Authorization", "Bearer "+t.beaconToken)
	} else if clone.Header.Get("Authorization") == "" && t.gridToken != "" {
		clone.Header.Set("Authorization", "Bearer "+t.gridToken)
	}

	return t.base.RoundTrip(clone)
}

func (t *tokenSwitchTransport) getCAPayload() []byte {
	t.caMu.RLock()
	defer t.caMu.RUnlock()
	if len(t.caPayload) == 0 {
		return nil
	}
	return append([]byte(nil), t.caPayload...)
}

func (t *tokenSwitchTransport) setCAPayload(payload []byte) {
	t.caMu.Lock()
	defer t.caMu.Unlock()
	t.caPayload = append([]byte(nil), payload...)
}

func (t *tokenSwitchTransport) BaseTransport() *http.Transport {
	if transport, ok := t.base.(*http.Transport); ok {
		return transport
	}
	return nil
}

func seedManifestCache(stateDir, endpoint, gridID string) error {
	trimmedEndpoint := strings.TrimSpace(endpoint)
	if trimmedEndpoint == "" {
		return fmt.Errorf("manifest endpoint is empty")
	}
	parsed, err := url.Parse(trimmedEndpoint)
	if err != nil {
		return fmt.Errorf("parse manifest endpoint: %w", err)
	}
	host := strings.TrimSpace(parsed.Host)
	if host == "" {
		return fmt.Errorf("manifest endpoint host missing")
	}
	hostOnly := host
	if withoutPort, _, err := net.SplitHostPort(host); err == nil {
		hostOnly = withoutPort
	}
	dir := filepath.Join(stateDir, "manifest")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create manifest cache dir: %w", err)
	}
	filename := strings.ReplaceAll(host, ":", "_")
	filename = strings.ReplaceAll(filename, "/", "_")
	cachePath := filepath.Join(dir, filename+".json")
	if _, err := os.Stat(cachePath); err == nil {
		return nil
	}
	now := time.Now().UTC()
	state := manifestCacheState{
		GridID:   strings.TrimSpace(gridID),
		Version:  1,
		KeyID:    "bootstrap",
		LastSync: now,
		Manifest: manifestSnapshot{
			GridID:      strings.TrimSpace(gridID),
			Version:     1,
			GeneratedAt: now,
			NextHop:     hostOnly,
			Entries: []manifestEntry{{
				Domain: hostOnly,
				NodeID: "bootstrap",
				GridID: strings.TrimSpace(gridID),
				Endpoints: []manifestEndpoint{{
					Address:  host,
					Protocol: strings.TrimSpace(parsed.Scheme),
				}},
				UpdatedAt: now,
			}},
		},
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest cache: %w", err)
	}
	if err := os.WriteFile(cachePath, payload, 0o644); err != nil {
		return fmt.Errorf("write manifest cache: %w", err)
	}
	return nil
}

type manifestCacheState struct {
	GridID    string           `json:"grid_id"`
	Version   uint64           `json:"version"`
	KeyID     string           `json:"key_id"`
	PublicKey string           `json:"public_key"`
	Manifest  manifestSnapshot `json:"manifest"`
	LastSync  time.Time        `json:"last_sync"`
}

type manifestSnapshot struct {
	GridID      string          `json:"grid_id"`
	Version     uint64          `json:"version"`
	GeneratedAt time.Time       `json:"generated_at"`
	Entries     []manifestEntry `json:"entries"`
	NextHop     string          `json:"next_hop,omitempty"`
}

type manifestEntry struct {
	Domain    string             `json:"domain"`
	NodeID    string             `json:"node_id"`
	GridID    string             `json:"grid_id"`
	Endpoints []manifestEndpoint `json:"endpoints"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type manifestEndpoint struct {
	Address  string `json:"address"`
	Protocol string `json:"protocol"`
}

func fetchGridCAPayload(endpoint, host, token string) ([]byte, error) {
	trimmedEndpoint := strings.TrimSpace(endpoint)
	if trimmedEndpoint == "" {
		return nil, fmt.Errorf("grid endpoint is empty")
	}
	target := strings.TrimRight(trimmedEndpoint, "/") + "/v1/tls/ca"
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build ca request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	if trimmedHost := strings.TrimSpace(host); trimmedHost != "" {
		req.Host = trimmedHost
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request ca: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("unexpected ca status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ca response: %w", err)
	}
	return payload, nil
}

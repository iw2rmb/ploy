package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/api/config"
)

// Assignment represents a control-plane assignment to be executed by the node.
type Assignment struct {
	ID      string
	Runtime string
	Payload map[string]any
}

// AssignmentExecutor executes control-plane assignments.
type AssignmentExecutor interface {
	Execute(ctx context.Context, assignment Assignment) error
}

// StatusProvider exposes the node status published back to the control plane.
type StatusProvider interface {
	Snapshot(ctx context.Context) (map[string]any, error)
}

// Options configure the control-plane client.
type Options struct {
	Config     config.ControlPlaneConfig
	Executor   AssignmentExecutor
	Status     StatusProvider
	HTTPClient *http.Client
}

// Client coordinates control-plane polling and status updates.
type Client struct {
	mu         sync.Mutex
	cfg        config.ControlPlaneConfig
	executor   AssignmentExecutor
	status     StatusProvider
	httpClient *http.Client
	running    bool
	loopCtx    context.Context
	cancel     context.CancelFunc
	group      sync.WaitGroup
}

// New constructs the control plane client.
func New(opts Options) (*Client, error) {
	if opts.Executor == nil {
		return nil, errors.New("controlplane: executor required")
	}
	if opts.Status == nil {
		opts.Status = noopStatusProvider{}
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		cfg:        opts.Config,
		executor:   opts.Executor,
		status:     opts.Status,
		httpClient: client,
	}, nil
}

// Start launches background polling routines.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return errors.New("controlplane: already running")
	}
	c.loopCtx, c.cancel = context.WithCancel(ctx)
	c.running = true
	c.group.Add(2)
	go c.assignmentLoop()
	go c.statusLoop()
	return nil
}

// Stop terminates the polling routines.
func (c *Client) Stop(ctx context.Context) error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	cancel := c.cancel
	c.cancel = nil
	c.running = false
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		c.group.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Reload updates the client configuration.
func (c *Client) Reload(ctx context.Context, cfg config.ControlPlaneConfig) error {
	_ = ctx
	c.mu.Lock()
	c.cfg = cfg
	c.mu.Unlock()
	return nil
}

// Config returns the current configuration snapshot.
func (c *Client) Config() config.ControlPlaneConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

func (c *Client) assignmentLoop() {
	defer c.group.Done()
	for {
		cfg, loopCtx := c.currentState()
		if loopCtx.Err() != nil {
			return
		}
		c.fetchAssignments(loopCtx, cfg)

		interval := cfg.AssignmentPollInterval
		if interval <= 0 {
			interval = 5 * time.Second
		}
		timer := time.NewTimer(interval)
		select {
		case <-loopCtx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *Client) statusLoop() {
	defer c.group.Done()
	for {
		cfg, loopCtx := c.currentState()
		if loopCtx.Err() != nil {
			return
		}
		c.publishStatus(loopCtx, cfg)
		interval := cfg.StatusPublishInterval
		if interval <= 0 {
			interval = 30 * time.Second
		}
		timer := time.NewTimer(interval)
		select {
		case <-loopCtx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *Client) currentState() (config.ControlPlaneConfig, context.Context) {
	c.mu.Lock()
	cfg := c.cfg
	loopCtx := c.loopCtx
	c.mu.Unlock()
	return cfg, loopCtx
}

func (c *Client) fetchAssignments(ctx context.Context, cfg config.ControlPlaneConfig) {
	endpoint, err := c.buildURL(cfg.Endpoint, cfg.AssignmentsEndpoint)
	if err != nil {
		return
	}
	values := url.Values{}
	if cfg.NodeID != "" {
		values.Set("node_id", cfg.NodeID)
	}
	if len(values) > 0 {
		if strings.Contains(endpoint, "?") {
			endpoint = endpoint + "&" + values.Encode()
		} else {
			endpoint = endpoint + "?" + values.Encode()
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return
	}
	var payload struct {
		Assignments []struct {
			ID      string         `json:"id"`
			Runtime string         `json:"runtime"`
			Payload map[string]any `json:"payload"`
		} `json:"assignments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return
	}
	for _, item := range payload.Assignments {
		a := Assignment{ID: strings.TrimSpace(item.ID), Runtime: strings.TrimSpace(item.Runtime), Payload: item.Payload}
		if err := c.executor.Execute(ctx, a); err != nil {
			continue
		}
	}
}

func (c *Client) publishStatus(ctx context.Context, cfg config.ControlPlaneConfig) {
	endpoint, err := c.buildURL(cfg.Endpoint, cfg.NodeStatusEndpoint)
	if err != nil {
		return
	}
	pathWithID := strings.TrimSuffix(endpoint, "/") + "/" + url.PathEscape(cfg.NodeID)
	status, err := c.status.Snapshot(ctx)
	if err != nil {
		return
	}
	body, err := json.Marshal(status)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, pathWithID, strings.NewReader(string(body)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (c *Client) buildURL(base, suffix string) (string, error) {
	if strings.TrimSpace(base) == "" {
		return "", errors.New("controlplane: endpoint missing")
	}
	if strings.TrimSpace(suffix) == "" {
		return strings.TrimRight(base, "/"), nil
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(suffix, "http://") || strings.HasPrefix(suffix, "https://") {
		return suffix, nil
	}
	joined := path.Join(baseURL.Path, suffix)
	baseURL.Path = joined
	return baseURL.String(), nil
}

type noopStatusProvider struct{}

func (noopStatusProvider) Snapshot(context.Context) (map[string]any, error) {
	return map[string]any{"state": "ok"}, nil
}

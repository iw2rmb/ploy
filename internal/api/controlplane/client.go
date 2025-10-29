package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/node/logstream"
)

// Assignment represents a control-plane assignment to be executed by the node.
type Assignment struct {
	ID       string
	Ticket   string
	StepID   string
	Runtime  string
	Metadata map[string]string
	Payload  map[string]any
}

// AssignmentError captures executor error metadata.
type AssignmentError struct {
	Reason  string
	Message string
}

// AssignmentResult summarises the outcome produced by an executor.
type AssignmentResult struct {
	State      string
	Error      *AssignmentError
	Artifacts  map[string]string
	Bundles    map[string]scheduler.BundleRecord
	Shift      *scheduler.ShiftMetrics
	Inspection bool
	Retention  *logstream.RetentionHint
}

// AssignmentExecutor executes control-plane assignments.
type AssignmentExecutor interface {
	Execute(ctx context.Context, assignment Assignment) (AssignmentResult, error)
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
	mu          sync.Mutex
	cfg         config.ControlPlaneConfig
	executor    AssignmentExecutor
	status      StatusProvider
	httpClient  *http.Client
	running     bool
	loopCtx     context.Context
	cancel      context.CancelFunc
	group       sync.WaitGroup
	emptyClaims int64
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
	go c.claimLoop()
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

// claimLoop continuously claims jobs and processes them sequentially.
func (c *Client) claimLoop() {
	defer c.group.Done()
	var bo claimBackoff
	for {
		cfg, loopCtx := c.currentState()
		if loopCtx == nil || loopCtx.Err() != nil {
			return
		}
		initial, max := c.backoffDurations(cfg)
		bo.configure(initial, max)

		job, err := c.claimJob(loopCtx, cfg)
		switch {
		case err != nil:
			log.Printf("controlplane: claim loop error (node=%s): %v", cfg.NodeID, err)
			atomic.StoreInt64(&c.emptyClaims, 0)
			wait := bo.next()
			if !sleepWithContext(loopCtx, wait) {
				return
			}
			continue
		case job == nil:
			count := atomic.AddInt64(&c.emptyClaims, 1)
			if count == 1 || count%50 == 0 {
				log.Printf("controlplane: claim returned empty (node=%s, streak=%d)", cfg.NodeID, count)
			}
			wait := bo.next()
			if !sleepWithContext(loopCtx, wait) {
				return
			}
			continue
		default:
			atomic.StoreInt64(&c.emptyClaims, 0)
			bo.reset()
			c.executeJob(loopCtx, cfg, *job)
		}
	}
}

// statusLoop periodically publishes node status to the control plane.
func (c *Client) statusLoop() {
	defer c.group.Done()
	for {
		cfg, loopCtx := c.currentState()
		if loopCtx == nil || loopCtx.Err() != nil {
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

// currentState returns the latest config snapshot and root context.
func (c *Client) currentState() (config.ControlPlaneConfig, context.Context) {
	c.mu.Lock()
	cfg := c.cfg
	loopCtx := c.loopCtx
	c.mu.Unlock()
	return cfg, loopCtx
}

// claimJob attempts to claim the next available job for the node.
func (c *Client) claimJob(ctx context.Context, cfg config.ControlPlaneConfig) (*claimedJob, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	endpoint, err := c.buildURL(cfg.Endpoint, c.claimEndpoint(cfg))
	if err != nil {
		return nil, err
	}
	payload := map[string]string{"node_id": cfg.NodeID}
	req, err := c.newJSONRequest(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusInternalServerError {
		log.Printf("controlplane: claim transient %s (node=%s)", resp.Status, cfg.NodeID)
		return nil, fmt.Errorf("controlplane: claim transient status %s", resp.Status)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		log.Printf("controlplane: claim rejected %s (node=%s)", resp.Status, cfg.NodeID)
		return nil, fmt.Errorf("controlplane: claim rejected with status %s", resp.Status)
	}

	var body claimResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		log.Printf("controlplane: decode claim response failed (node=%s): %v", cfg.NodeID, err)
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(body.Status)) {
	case "", "empty":
		return nil, nil
	case "claimed":
		if body.Job.ID == "" {
			return nil, errors.New("controlplane: claim missing job id")
		}
		job := claimedJob{
			ID:       strings.TrimSpace(body.Job.ID),
			Ticket:   strings.TrimSpace(body.Job.Ticket),
			StepID:   strings.TrimSpace(body.Job.StepID),
			Priority: strings.TrimSpace(body.Job.Priority),
			Metadata: cloneMetadata(body.Job.Metadata),
		}
		log.Printf("controlplane: claimed job %s ticket=%s (node=%s)", job.ID, job.Ticket, cfg.NodeID)
		return &job, nil
	default:
		log.Printf("controlplane: unexpected claim status %q (node=%s)", body.Status, cfg.NodeID)
		return nil, fmt.Errorf("controlplane: unexpected claim status %q", body.Status)
	}
}

// executeJob runs the executor and coordinates heartbeats and completion.
func (c *Client) executeJob(loopCtx context.Context, cfg config.ControlPlaneConfig, job claimedJob) {
	if loopCtx == nil || loopCtx.Err() != nil {
		return
	}
	assignment := c.assignmentFromJob(job)
	c.appendLog(loopCtx, cfg, job, "stdout", fmt.Sprintf("starting job %s on node %s", job.ID, cfg.NodeID))
	if err := c.sendHeartbeat(loopCtx, cfg, job); err != nil && loopCtx.Err() == nil {
		log.Printf("controlplane: initial heartbeat for job %s failed: %v", job.ID, err)
	}

	jobCtx, jobCancel := context.WithCancel(loopCtx)
	defer jobCancel()

	heartbeatCtx, heartbeatCancel := context.WithCancel(jobCtx)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		c.heartbeatLoop(heartbeatCtx, cfg, job)
	}()

	result, execErr := c.executor.Execute(jobCtx, assignment)

	heartbeatCancel()
	<-heartbeatDone

	if loopCtx.Err() != nil {
		return
	}

	var (
		state  = jobStateSucceeded
		jobErr *jobErrorPayload
	)
	if trimmed := strings.TrimSpace(result.State); trimmed != "" {
		state = trimmed
	} else if execErr == nil {
		state = jobStateSucceeded
	}
	if execErr != nil {
		if state == "" {
			state = jobStateFailed
		}
		c.appendLog(loopCtx, cfg, job, "stderr", fmt.Sprintf("job %s failed: %v", job.ID, execErr))
	} else {
		c.appendLog(loopCtx, cfg, job, "stdout", fmt.Sprintf("job %s completed successfully", job.ID))
	}

	if result.Error != nil {
		jobErr = &jobErrorPayload{
			Reason:  strings.TrimSpace(result.Error.Reason),
			Message: strings.TrimSpace(result.Error.Message),
		}
	} else if execErr != nil {
		reason := "executor_error"
		if errors.Is(execErr, context.Canceled) {
			reason = "executor_canceled"
		}
		jobErr = &jobErrorPayload{
			Reason:  reason,
			Message: execErr.Error(),
		}
	}

	if err := c.completeWithRetry(loopCtx, cfg, job, state, jobErr, result); err != nil {
		log.Printf("controlplane: complete job %s: %v", job.ID, err)
	}
}

// heartbeatLoop sends periodic heartbeats for the running job.
func (c *Client) heartbeatLoop(ctx context.Context, cfg config.ControlPlaneConfig, job claimedJob) {
	interval := cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if err := c.sendHeartbeat(ctx, cfg, job); err != nil && ctx.Err() == nil {
			log.Printf("controlplane: heartbeat for job %s failed: %v", job.ID, err)
		}
	}
}

// sendHeartbeat posts a heartbeat for the specified job.
func (c *Client) sendHeartbeat(ctx context.Context, cfg config.ControlPlaneConfig, job claimedJob) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	endpoint, err := c.jobActionURL(cfg.Endpoint, c.heartbeatEndpoint(cfg), job.ID, "heartbeat")
	if err != nil {
		return err
	}
	payload := map[string]string{
		"ticket":  job.Ticket,
		"node_id": cfg.NodeID,
	}
	req, err := c.newJSONRequest(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("controlplane: heartbeat status %s", resp.Status)
	}
	return nil
}

// completeWithRetry attempts to complete the job, retrying transient failures.
func (c *Client) completeWithRetry(ctx context.Context, cfg config.ControlPlaneConfig, job claimedJob, state string, jobErr *jobErrorPayload, result AssignmentResult) error {
	var (
		attempts     int
		initial, max = c.backoffDurations(cfg)
		bo           claimBackoff
	)
	bo.configure(initial, max)
	for {
		retryable, err := c.postCompletion(ctx, cfg, job, state, jobErr, result)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		attempts++
		if !retryable || attempts >= 5 {
			return err
		}
		wait := bo.next()
		if !sleepWithContext(ctx, wait) {
			return ctx.Err()
		}
	}
}

// postCompletion posts the completion payload and indicates whether to retry.
func (c *Client) postCompletion(ctx context.Context, cfg config.ControlPlaneConfig, job claimedJob, state string, jobErr *jobErrorPayload, result AssignmentResult) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	endpoint, err := c.jobActionURL(cfg.Endpoint, c.completionEndpoint(cfg), job.ID, "complete")
	if err != nil {
		return true, err
	}
	payload := completionRequest{
		Ticket:     job.Ticket,
		NodeID:     cfg.NodeID,
		State:      state,
		Error:      jobErr,
		Artifacts:  cloneMetadata(result.Artifacts),
		Bundles:    cloneBundleRecords(result.Bundles),
		Inspection: result.Inspection,
	}
	if result.Shift != nil {
		payload.Shift = &shiftPayload{
			Result:          strings.TrimSpace(result.Shift.Result),
			DurationSeconds: result.Shift.Duration.Seconds(),
		}
	}
	req, err := c.newJSONRequest(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return true, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= http.StatusInternalServerError:
		return true, fmt.Errorf("controlplane: completion transient status %s", resp.Status)
	case resp.StatusCode >= http.StatusBadRequest:
		return false, fmt.Errorf("controlplane: completion rejected with status %s", resp.Status)
	default:
		return false, nil
	}
}

func (c *Client) postJobLog(ctx context.Context, cfg config.ControlPlaneConfig, job claimedJob, req jobLogRequest) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	endpoint, err := c.jobActionURL(cfg.Endpoint, c.logEndpoint(cfg), job.ID, "logs/entries")
	if err != nil {
		return err
	}
	payload := req
	payload.Ticket = strings.TrimSpace(payload.Ticket)
	if payload.Ticket == "" {
		payload.Ticket = job.Ticket
	}
	payload.NodeID = strings.TrimSpace(payload.NodeID)
	if payload.NodeID == "" {
		payload.NodeID = cfg.NodeID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("controlplane: log append returned %s", resp.Status)
	}
	return nil
}

// assignmentFromJob maps the claimed job payload into an assignment struct.
func (c *Client) assignmentFromJob(job claimedJob) Assignment {
	payload := map[string]any{
		"ticket":   job.Ticket,
		"step_id":  job.StepID,
		"priority": job.Priority,
	}
	for k, v := range job.Metadata {
		payload[k] = v
	}
	return Assignment{
		ID:       job.ID,
		Ticket:   job.Ticket,
		StepID:   job.StepID,
		Runtime:  strings.TrimSpace(job.Metadata["runtime"]),
		Metadata: cloneMetadata(job.Metadata),
		Payload:  payload,
	}
}

const (
	jobStateSucceeded = "succeeded"
	jobStateFailed    = "failed"
)

// claimedJob describes the subset of job data needed by the worker client.
type claimedJob struct {
	ID       string
	Ticket   string
	StepID   string
	Priority string
	Metadata map[string]string
}

// claimResponse models the claim endpoint payload.
type claimResponse struct {
	Status string        `json:"status"`
	NodeID string        `json:"node_id"`
	Job    claimJobModel `json:"job"`
}

// claimJobModel captures the job payload embedded in claim responses.
type claimJobModel struct {
	ID       string            `json:"id"`
	Ticket   string            `json:"ticket"`
	StepID   string            `json:"step_id"`
	Priority string            `json:"priority"`
	Metadata map[string]string `json:"metadata"`
}

// jobErrorPayload represents execution failure metadata sent during completion.
type jobErrorPayload struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// completionRequest is the JSON payload posted to the completion endpoint.
type completionRequest struct {
	Ticket     string                            `json:"ticket"`
	NodeID     string                            `json:"node_id"`
	State      string                            `json:"state"`
	Error      *jobErrorPayload                  `json:"error,omitempty"`
	Artifacts  map[string]string                 `json:"artifacts,omitempty"`
	Bundles    map[string]scheduler.BundleRecord `json:"bundles,omitempty"`
	Shift      *shiftPayload                     `json:"shift,omitempty"`
	Inspection bool                              `json:"inspection,omitempty"`
}

type shiftPayload struct {
	Result          string  `json:"result"`
	DurationSeconds float64 `json:"duration_seconds"`
}

type jobLogRequest struct {
	Ticket    string `json:"ticket"`
	NodeID    string `json:"node_id"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`
	Timestamp string `json:"timestamp"`
}

func (c *Client) appendLog(ctx context.Context, cfg config.ControlPlaneConfig, job claimedJob, stream, line string) {
	record := jobLogRequest{
		Ticket:    job.Ticket,
		NodeID:    cfg.NodeID,
		Stream:    strings.TrimSpace(stream),
		Line:      line,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if record.Stream == "" {
		record.Stream = "stdout"
	}
	if err := c.postJobLog(ctx, cfg, job, record); err != nil {
		log.Printf("controlplane: post log for job %s failed: %v", job.ID, err)
	}
}

// claimBackoff manages exponential backoff between claim attempts.
type claimBackoff struct {
	initial time.Duration
	max     time.Duration
	current time.Duration
}

// configure initialises the backoff bounds, resetting if they change.
func (b *claimBackoff) configure(initial, max time.Duration) {
	if initial <= 0 {
		initial = 100 * time.Millisecond
	}
	if max <= 0 {
		max = initial * 4
	}
	if max < initial {
		max = initial
	}
	if b.initial != initial || b.max != max {
		b.initial = initial
		b.max = max
		b.current = 0
	}
}

// next returns the next backoff duration without exceeding the configured max.
func (b *claimBackoff) next() time.Duration {
	if b.current <= 0 {
		b.current = b.initial
		return b.current
	}
	b.current *= 2
	if b.current > b.max {
		b.current = b.max
	}
	return b.current
}

// reset clears the accumulated backoff so the next call returns the initial interval.
func (b *claimBackoff) reset() {
	b.current = 0
}

func cloneBundleRecords(src map[string]scheduler.BundleRecord) map[string]scheduler.BundleRecord {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]scheduler.BundleRecord, len(src))
	for k, v := range src {
		dst[k] = scheduler.BundleRecord{
			CID:       strings.TrimSpace(v.CID),
			Digest:    strings.TrimSpace(v.Digest),
			Size:      v.Size,
			Retained:  v.Retained,
			TTL:       strings.TrimSpace(v.TTL),
			ExpiresAt: strings.TrimSpace(v.ExpiresAt),
		}
	}
	return dst
}

// cloneMetadata copies metadata maps for safe reuse.
func cloneMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// backoffDurations derives claim retry bounds from the control-plane config.
func (c *Client) backoffDurations(cfg config.ControlPlaneConfig) (time.Duration, time.Duration) {
	initial := cfg.InitialBackoff
	if initial <= 0 {
		initial = cfg.AssignmentPollInterval
	}
	if initial <= 0 {
		initial = 100 * time.Millisecond
	}
	max := cfg.MaxBackoff
	if max <= 0 {
		max = initial * 8
	}
	if max < initial {
		max = initial
	}
	return initial, max
}

// claimEndpoint selects the claim endpoint path, falling back to defaults.
func (c *Client) claimEndpoint(cfg config.ControlPlaneConfig) string {
	if trimmed := strings.TrimSpace(cfg.JobClaimEndpoint); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(cfg.AssignmentsEndpoint); trimmed != "" {
		return trimmed
	}
	return "/v1/jobs/claim"
}

// heartbeatEndpoint returns the base heartbeat path for job-specific routes.
func (c *Client) heartbeatEndpoint(cfg config.ControlPlaneConfig) string {
	if trimmed := strings.TrimSpace(cfg.JobHeartbeatEndpoint); trimmed != "" {
		return trimmed
	}
	return "/v1/jobs"
}

// completionEndpoint returns the base completion path for job-specific routes.
func (c *Client) completionEndpoint(cfg config.ControlPlaneConfig) string {
	if trimmed := strings.TrimSpace(cfg.JobCompleteEndpoint); trimmed != "" {
		return trimmed
	}
	return "/v1/jobs"
}

func (c *Client) logEndpoint(cfg config.ControlPlaneConfig) string {
	if trimmed := strings.TrimSpace(cfg.JobLogEndpoint); trimmed != "" {
		return trimmed
	}
	return "/v1/jobs"
}

// jobActionURL assembles a job-specific URL for the given action.
func (c *Client) jobActionURL(base, prefix, jobID, action string) (string, error) {
	endpoint, err := c.buildURL(base, prefix)
	if err != nil {
		return "", err
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/" + url.PathEscape(jobID)
	if action != "" {
		endpoint += "/" + strings.TrimPrefix(action, "/")
	}
	return endpoint, nil
}

// newJSONRequest constructs an HTTP request with a JSON body.
func (c *Client) newJSONRequest(ctx context.Context, method, endpoint string, payload any) (*http.Request, error) {
	var body *bytes.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// sleepWithContext waits for the given duration or until the context finishes.
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		d = 50 * time.Millisecond
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
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

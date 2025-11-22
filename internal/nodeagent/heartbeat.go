package nodeagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/iw2rmb/ploy/internal/worker/lifecycle"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// HeartbeatPayload contains resource snapshot data sent to the server.
type HeartbeatPayload struct {
	NodeID        string    `json:"node_id"`
	Timestamp     time.Time `json:"timestamp"`
	CPUFreeMilli  float64   `json:"cpu_free_millis"`
	MemFreeMB     float64   `json:"mem_free_mb"`
	DiskFreeMB    float64   `json:"disk_free_mb"`
	CPUTotalMilli float64   `json:"cpu_total_millis"`
	MemTotalMB    float64   `json:"mem_total_mb"`
	DiskTotalMB   float64   `json:"disk_total_mb"`
}

// HeartbeatManager periodically sends resource snapshots to the server.
type HeartbeatManager struct {
	cfg           Config
	collector     *lifecycle.Collector
	client        *http.Client
	backoff       *backoff.StatefulBackoff
	backoffActive bool // Tracks whether backoff is currently active (true after 5xx, false after success)
}

// NewHeartbeatManager constructs a heartbeat manager.
func NewHeartbeatManager(cfg Config) (*HeartbeatManager, error) {
	collector := lifecycle.NewCollector(lifecycle.Options{
		Role:   "node",
		NodeID: cfg.NodeID,
	})

	// Don't create HTTP client yet - defer until after bootstrap runs.
	// Client will be lazily initialized on first heartbeat.

	// Create stateful backoff using shared HeartbeatPolicy (5s initial, 5m max).
	backoffPolicy := backoff.HeartbeatPolicy()
	statefulBackoff := backoff.NewStatefulBackoff(backoffPolicy)

	return &HeartbeatManager{
		cfg:           cfg,
		collector:     collector,
		client:        nil,             // Will be initialized lazily
		backoff:       statefulBackoff, // Shared backoff helper for 5xx retry logic
		backoffActive: false,           // No backoff until first 5xx error
	}, nil
}

// Start begins sending heartbeats.
func (h *HeartbeatManager) Start(ctx context.Context) error {
	ticker := time.NewTicker(h.cfg.Heartbeat.Interval)
	defer ticker.Stop()

	// Send initial heartbeat.
	if err := h.sendHeartbeat(ctx); err != nil {
		slog.Error("initial heartbeat failed", "err", err)
		h.applyBackoff(err)
	} else {
		h.resetBackoff()
	}

	for {
		// If backoff is active (triggered by prior 5xx error), wait before next heartbeat.
		if h.backoffActive {
			backoffDuration := h.backoff.GetDuration()
			slog.Warn("heartbeat backoff active", "duration", backoffDuration)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoffDuration):
				// Backoff wait complete, continue.
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := h.sendHeartbeat(ctx); err != nil {
				slog.Error("heartbeat failed", "err", err)
				h.applyBackoff(err)
			} else {
				h.resetBackoff()
			}
		}
	}
}

func (h *HeartbeatManager) sendHeartbeat(ctx context.Context) error {
	// Lazy initialization: create HTTP client if not yet initialized.
	// This allows bootstrap() to run first and create certificates and bearer token.
	if h.client == nil {
		client, err := createHTTPClient(h.cfg)
		if err != nil {
			return fmt.Errorf("create http client: %w", err)
		}
		h.client = client
	}

	snap, err := h.collector.Collect(ctx)
	if err != nil {
		return fmt.Errorf("collect snapshot: %w", err)
	}

	// Use typed NodeCapacity instead of map[string]any casts.
	// This eliminates unsafe type assertions and provides compile-time safety.
	capacity := snap.Capacity

	payload := HeartbeatPayload{
		NodeID:        h.cfg.NodeID,
		Timestamp:     time.Now().UTC(),
		CPUFreeMilli:  capacity.CPUFreeMilli,
		CPUTotalMilli: capacity.CPUTotalMilli,
		MemFreeMB:     capacity.MemFreeMB,
		MemTotalMB:    capacity.MemTotalMB,
		DiskFreeMB:    capacity.DiskFreeMB,
		DiskTotalMB:   capacity.DiskTotalMB,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, h.cfg.Heartbeat.Timeout)
	defer cancel()

	hbURL, err := buildURL(h.cfg.ServerURL, path.Join("/v1/nodes", url.PathEscape(h.cfg.NodeID), "heartbeat"))
	if err != nil {
		return fmt.Errorf("build heartbeat url: %w", err)
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, hbURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("heartbeat failed with status %d", resp.StatusCode)
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			return &serverError{statusCode: resp.StatusCode, err: err}
		}
		return err
	}

	return nil
}

// serverError wraps a 5xx error for backoff handling.
type serverError struct {
	statusCode int
	err        error
}

func (e *serverError) Error() string {
	return e.err.Error()
}

func (e *serverError) Unwrap() error {
	return e.err
}

// applyBackoff triggers exponential backoff when a 5xx error occurs.
// Uses the shared StatefulBackoff helper to compute the next backoff duration.
// Only 5xx errors (wrapped in serverError) trigger backoff; other errors are ignored.
func (h *HeartbeatManager) applyBackoff(err error) {
	var srvErr *serverError
	// Only apply backoff for 5xx server errors (preserves existing 5xx-only semantics).
	if err != nil && errors.As(err, &srvErr) {
		h.backoff.Apply()      // Advance to next backoff interval (5s, 10s, 20s, ..., 5m).
		h.backoffActive = true // Mark backoff as active so the loop waits.
	}
}

// resetBackoff clears backoff state on successful heartbeat.
// Resets the StatefulBackoff to initial interval and deactivates backoff.
func (h *HeartbeatManager) resetBackoff() {
	h.backoff.Reset()       // Reset shared backoff helper to initial state.
	h.backoffActive = false // Deactivate backoff (no wait on next loop iteration).
}

// buildURL resolves a base URL and a path, preserving scheme/host.
func buildURL(base, p string) (string, error) {
	bu, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	pu, err := url.Parse(p)
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}
	return bu.ResolveReference(pu).String(), nil
}

func newHTTPClient(cfg Config) (*http.Client, error) {
	if !cfg.HTTP.TLS.Enabled {
		return &http.Client{Timeout: 30 * time.Second}, nil
	}

	// Load node certificate and key for client authentication.
	cert, err := tls.LoadX509KeyPair(cfg.HTTP.TLS.CertPath, cfg.HTTP.TLS.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("load certificate: %w", err)
	}

	// Load CA certificate for server verification.
	caData, err := os.ReadFile(cfg.HTTP.TLS.CAPath)
	if err != nil {
		return nil, fmt.Errorf("load ca certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("failed to parse ca certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}, nil
}

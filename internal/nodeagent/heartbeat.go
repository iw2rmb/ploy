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
	cfg             Config
	collector       *lifecycle.Collector
	client          *http.Client
	backoffDuration time.Duration
	maxBackoff      time.Duration
}

// NewHeartbeatManager constructs a heartbeat manager.
func NewHeartbeatManager(cfg Config) (*HeartbeatManager, error) {
	collector := lifecycle.NewCollector(lifecycle.Options{
		Role:   "node",
		NodeID: cfg.NodeID,
	})

	client, err := newHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	return &HeartbeatManager{
		cfg:             cfg,
		collector:       collector,
		client:          client,
		backoffDuration: 0,
		maxBackoff:      5 * time.Minute,
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
		// If backoff is active, wait for it.
		if h.backoffDuration > 0 {
			slog.Warn("heartbeat backoff active", "duration", h.backoffDuration)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(h.backoffDuration):
				// Backoff complete, continue.
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
	snap, err := h.collector.Collect(ctx)
	if err != nil {
		return fmt.Errorf("collect snapshot: %w", err)
	}

	resources := snap.Status["resources"].(map[string]any)
	cpu := resources["cpu"].(map[string]any)
	memory := resources["memory"].(map[string]any)
	disk := resources["disk"].(map[string]any)

	payload := HeartbeatPayload{
		NodeID:        h.cfg.NodeID,
		Timestamp:     time.Now().UTC(),
		CPUFreeMilli:  cpu["free_mcores"].(float64),
		CPUTotalMilli: cpu["total_mcores"].(float64),
		MemFreeMB:     memory["free_mb"].(float64),
		MemTotalMB:    memory["total_mb"].(float64),
		DiskFreeMB:    disk["free_mb"].(float64),
		DiskTotalMB:   disk["total_mb"].(float64),
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

// applyBackoff increases backoff duration when a 5xx error occurs.
func (h *HeartbeatManager) applyBackoff(err error) {
	var srvErr *serverError
	if err != nil && errors.As(err, &srvErr) {
		if h.backoffDuration == 0 {
			h.backoffDuration = 5 * time.Second
		} else {
			h.backoffDuration = h.backoffDuration * 2
			if h.backoffDuration > h.maxBackoff {
				h.backoffDuration = h.maxBackoff
			}
		}
	}
}

// resetBackoff clears backoff duration on successful heartbeat.
func (h *HeartbeatManager) resetBackoff() {
	h.backoffDuration = 0
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

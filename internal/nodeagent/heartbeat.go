package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/worker/lifecycle"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// HeartbeatPayload contains resource snapshot data sent to the server.
type HeartbeatPayload struct {
	CPUFreeMillis  int32  `json:"cpu_free_millis"`
	CPUTotalMillis int32  `json:"cpu_total_millis"`
	MemFreeBytes   int64  `json:"mem_free_bytes"`
	MemTotalBytes  int64  `json:"mem_total_bytes"`
	DiskFreeBytes  int64  `json:"disk_free_bytes"`
	DiskTotalBytes int64  `json:"disk_total_bytes"`
	Version        string `json:"version,omitempty"`
}

// HeartbeatManager periodically sends resource snapshots to the server.
type HeartbeatManager struct {
	cfg           Config
	collector     *lifecycle.Collector
	client        *http.Client
	clientOnce    sync.Once // Ensures thread-safe lazy HTTP client initialization
	clientErr     error     // Stores initialization error from clientOnce
	backoff       *backoff.StatefulBackoff
	backoffActive bool // Tracks whether backoff is currently active (true after 5xx, false after success)
}

// NewHeartbeatManager constructs a heartbeat manager.
func NewHeartbeatManager(cfg Config) (*HeartbeatManager, error) {
	// Read PLOY_LIFECYCLE_NET_IGNORE env var and parse comma-separated patterns.
	// This allows operators to ignore noisy network interfaces (e.g., docker*, veth*, cni*)
	// when computing throughput metrics. Empty patterns are filtered out.
	ignore := []string{}
	if raw := os.Getenv("PLOY_LIFECYCLE_NET_IGNORE"); strings.TrimSpace(raw) != "" {
		for _, pattern := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(pattern); trimmed != "" {
				ignore = append(ignore, trimmed)
			}
		}
	}

	collector, err := lifecycle.NewCollector(lifecycle.Options{
		Role:             "node",
		NodeID:           cfg.NodeID,
		IgnoreInterfaces: ignore,
	})
	if err != nil {
		return nil, fmt.Errorf("new lifecycle collector: %w", err)
	}

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
	// Send initial heartbeat.
	if err := h.sendHeartbeat(ctx); err != nil {
		slog.Error("initial heartbeat failed", "err", err)
		h.applyBackoff(err)
	} else {
		h.resetBackoff()
	}

	// Use a single timer to schedule both steady-state intervals and backoff delays.
	// This avoids ticker drift/dropped ticks when a backoff sleep occurs, and avoids
	// allocating a new timer on each backoff via time.After.
	timer := time.NewTimer(0)
	defer timer.Stop()
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}

	for {
		delay := h.cfg.Heartbeat.Interval

		// If backoff is active (triggered by prior 5xx error), wait before next heartbeat.
		if h.backoffActive {
			delay = time.Duration(h.backoff.GetDuration())
			slog.Warn("heartbeat backoff active", "duration", delay)
		}

		if delay < 0 {
			delay = 0
		}

		timer.Reset(delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
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
	// Uses sync.Once for thread-safe initialization.
	h.clientOnce.Do(func() {
		h.client, h.clientErr = createHTTPClient(h.cfg)
	})
	if h.clientErr != nil {
		return fmt.Errorf("create http client: %w", h.clientErr)
	}

	snap, err := h.collector.Collect(ctx)
	if err != nil {
		return fmt.Errorf("collect snapshot: %w", err)
	}

	// Use typed NodeCapacity instead of map[string]any casts.
	// This eliminates unsafe type assertions and provides compile-time safety.
	capacity := snap.Capacity

	payload := HeartbeatPayload{
		CPUFreeMillis:  int32(capacity.CPUFreeMillis),
		CPUTotalMillis: int32(capacity.CPUTotalMillis),
		MemFreeBytes:   int64(capacity.MemFreeBytes),
		MemTotalBytes:  int64(capacity.MemTotalBytes),
		DiskFreeBytes:  int64(capacity.DiskFreeBytes),
		DiskTotalBytes: int64(capacity.DiskTotalBytes),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, h.cfg.Heartbeat.Timeout)
	defer cancel()

	hbURL, err := BuildURL(h.cfg.ServerURL, path.Join("/v1/nodes", url.PathEscape(h.cfg.NodeID.String()), "heartbeat"))
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

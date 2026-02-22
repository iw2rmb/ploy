package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

const (
	// flushInterval is how often to flush buffered logs to the server.
	flushInterval = 2 * time.Second
)

// Aliases for compression constants - keeps local usage clear.
const (
	maxChunkSize  = MaxUploadSize
	softChunkSize = SoftUploadSize
)

// LogHook is a function that processes log data before it is
// compressed and sent to the server. A nil LogHook is treated as a no-op.
type LogHook func(p []byte) ([]byte, error)

// LogStreamer buffers logs and streams them as gzipped chunks to the server.
type LogStreamer struct {
	cfg        Config
	runID      types.RunID
	jobID      types.JobID
	chunkNo    int32
	buffer     bytes.Buffer
	gzWriter   *gzip.Writer
	mu         sync.Mutex
	flushDone  chan struct{}
	closeOnce  sync.Once
	stopCh     chan struct{}
	closed     bool    // Set to true when Close() is called to prevent sending during shutdown.
	hook       LogHook // Optional hook to process logs before compression.
	httpClient *http.Client
}

// NewLogStreamer creates a new log streamer for a specific run and (optionally) job.
// Returns an error if HTTP client creation fails (e.g., missing bearer token).
func NewLogStreamer(cfg Config, runID types.RunID, jobID types.JobID) (*LogStreamer, error) {
	ls := &LogStreamer{
		cfg:       cfg,
		runID:     runID,
		jobID:     jobID,
		chunkNo:   0,
		flushDone: make(chan struct{}),
		stopCh:    make(chan struct{}),
	}
	ls.gzWriter = gzip.NewWriter(&ls.buffer)

	// Initialize HTTP client (honors mTLS when enabled in cfg).
	// Fail hard if client creation fails - a fallback to unauthenticated client
	// would silently fail all uploads, causing log data loss.
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create HTTP client for log streamer: %w", err)
	}
	ls.httpClient = client

	// Start background flusher.
	go ls.periodicFlush()

	return ls, nil
}

// SetHook sets the log processing hook. Must be called before any writes.
// This method is not safe for concurrent use with Write.
func (ls *LogStreamer) SetHook(hook LogHook) {
	ls.hook = hook
}

// Write implements io.Writer interface for capturing logs.
func (ls *LogStreamer) Write(p []byte) (n int, err error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Apply hook to process log data (e.g., scrub PII).
	processed := p
	if ls.hook != nil {
		var hookErr error
		processed, hookErr = ls.hook(p)
		if hookErr != nil {
			slog.Warn("log hook failed, using original data", "run_id", ls.runID, "error", hookErr)
			processed = p // Fall back to original on error.
		} else if processed == nil {
			// Defensive: a misbehaving hook returned nil without error; write original
			// data to preserve io.Writer semantics (n == len(p)).
			slog.Warn("log hook returned nil data; using original", "run_id", ls.runID)
			processed = p
		}
	}

	// Write to gzip writer.
	_, err = ls.gzWriter.Write(processed)
	if err != nil {
		return 0, fmt.Errorf("write to gzip: %w", err)
	}

	// Check if we need to flush due to size.
	if err := ls.gzWriter.Flush(); err != nil {
		return 0, fmt.Errorf("flush gzip: %w", err)
	}

	// Use soft threshold to ensure finalized (Close) chunk stays under hard cap.
	if ls.buffer.Len() >= softChunkSize {
		if flushErr := ls.flushLocked(); flushErr != nil {
			slog.Warn("log streamer flush failed", "run_id", ls.runID, "error", flushErr)
		}
	}

	// Return the number of bytes consumed from the original input.
	return len(p), nil
}

// periodicFlush runs in the background and flushes buffered logs periodically.
func (ls *LogStreamer) periodicFlush() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ls.mu.Lock()
			if ls.buffer.Len() > 0 {
				if err := ls.flushLocked(); err != nil {
					slog.Warn("periodic flush failed", "run_id", ls.runID, "error", err)
				}
			}
			ls.mu.Unlock()
		case <-ls.stopCh:
			close(ls.flushDone)
			return
		}
	}
}

// flushLocked flushes the current buffer to the server. Must be called with ls.mu held.
func (ls *LogStreamer) flushLocked() error {
	if ls.buffer.Len() == 0 {
		return nil
	}

	// Close the gzip writer to finalize the compressed data.
	// Always create a new writer afterwards, even on error, to prevent double-close
	// if Close() is called later.
	closeErr := ls.gzWriter.Close()
	if closeErr != nil {
		ls.buffer.Reset()
		ls.gzWriter = gzip.NewWriter(&ls.buffer)
		return fmt.Errorf("close gzip writer: %w", closeErr)
	}

	// Get the compressed data.
	compressed := make([]byte, ls.buffer.Len())
	copy(compressed, ls.buffer.Bytes())

	// Enforce size cap.
	if len(compressed) > maxChunkSize {
		// Drop the oversize payload to preserve forward progress but report an error.
		// Reset state so subsequent writes proceed with a fresh chunk.
		// Log at error level since data is permanently lost.
		slog.Error("log chunk dropped due to size limit",
			"run_id", ls.runID,
			"job_id", ls.jobID,
			"size", len(compressed),
			"limit", maxChunkSize,
		)
		ls.buffer.Reset()
		ls.gzWriter = gzip.NewWriter(&ls.buffer)
		return fmt.Errorf("compressed chunk exceeds 10 MiB: %d bytes (data dropped)", len(compressed))
	}

	// Reset buffer and gzip writer for next chunk.
	ls.buffer.Reset()
	ls.gzWriter = gzip.NewWriter(&ls.buffer)

	// Increment chunk number.
	currentChunkNo := ls.chunkNo
	ls.chunkNo++

	// Check if Close() was called - if so, skip sending to avoid race condition.
	if ls.closed {
		return errors.New("log streamer closed during flush")
	}

	// Send to server (release lock during network call).
	ls.mu.Unlock()
	err := ls.sendChunk(compressed, currentChunkNo)
	ls.mu.Lock()

	// Check again after re-acquiring lock in case Close() was called during send.
	if ls.closed {
		return errors.New("log streamer closed during send")
	}

	return err
}

// sendChunk sends a gzipped log chunk to the server.
func (ls *LogStreamer) sendChunk(data []byte, chunkNo int32) error {
	if len(data) == 0 {
		return nil
	}

	// Use cfg.NodeID directly as a string. Node IDs are NanoID(6) strings
	// that don't require UUID parsing.
	nodeID := ls.cfg.NodeID

	// Prepare request payload.
	payload := struct {
		RunID   types.RunID  `json:"run_id"`
		JobID   *types.JobID `json:"job_id,omitempty"`
		ChunkNo int32        `json:"chunk_no"`
		Data    []byte       `json:"data"`
	}{
		RunID:   ls.runID,
		ChunkNo: chunkNo,
		Data:    data,
	}
	if !ls.jobID.IsZero() {
		payload.JobID = &ls.jobID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// Send to server endpoint using the node ID string directly.
	apiPath := fmt.Sprintf("/v1/nodes/%s/logs", nodeID.String())
	url := MustBuildURL(ls.cfg.ServerURL, apiPath)
	// Create per-request context with timeout. We intentionally use context.Background()
	// rather than a parent context because:
	// 1. Log uploads should complete even if the job context is canceled
	// 2. Close() must be able to flush final logs after job termination
	// 3. Each request has its own timeout to prevent indefinite blocking
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := ls.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	slog.Debug("log chunk sent", "run_id", ls.runID, "chunk_no", chunkNo, "size", len(data))
	return nil
}

// Close flushes any remaining logs and stops the streamer.
// Returns all errors encountered during close using errors.Join.
func (ls *LogStreamer) Close() error {
	var closeErr error
	ls.closeOnce.Do(func() {
		// Stop the background flusher.
		close(ls.stopCh)
		<-ls.flushDone

		// Flush any remaining logs.
		ls.mu.Lock()
		defer ls.mu.Unlock()

		var errs []error
		if ls.buffer.Len() > 0 {
			if err := ls.flushLocked(); err != nil {
				errs = append(errs, err)
			}
		}

		// Mark as closed to prevent any pending operations from sending.
		ls.closed = true

		// Close the gzip writer.
		if err := ls.gzWriter.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close gzip writer: %w", err))
		}

		closeErr = errors.Join(errs...)
	})

	return closeErr
}

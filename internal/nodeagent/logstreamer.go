package nodeagent

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

const (
	// maxChunkSize is the maximum size of a gzipped log chunk (1 MiB).
	maxChunkSize = 1 << 20
	// softChunkSize provides headroom for gzip footer so finalized chunks
	// never exceed the hard cap. Keep conservative margin.
	softChunkSize = maxChunkSize - 64
	// flushInterval is how often to flush buffered logs to the server.
	flushInterval = 2 * time.Second
)

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
	hook       LogHook // Optional hook to process logs before compression.
	httpClient *http.Client
}

// NewLogStreamer creates a new log streamer for a specific run and (optionally) job.
// By default, uses NoOpLogHook (no PII scrubbing).
func NewLogStreamer(cfg Config, runID types.RunID, jobID types.JobID) *LogStreamer {
	ls := &LogStreamer{
		cfg:       cfg,
		runID:     runID,
		jobID:     jobID,
		chunkNo:   0,
		flushDone: make(chan struct{}),
		stopCh:    make(chan struct{}),
		hook:      &NoOpLogHook{}, // Default to no-op hook.
	}
	ls.gzWriter = gzip.NewWriter(&ls.buffer)

	// Initialize HTTP client (honors mTLS when enabled in cfg).
	if c, err := createHTTPClient(cfg); err == nil {
		ls.httpClient = c
	} else {
		slog.Warn("log streamer: create HTTP client failed; using default", "run_id", runID, "error", err)
		ls.httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	// Start background flusher.
	go ls.periodicFlush()

	return ls
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
		processed, hookErr = ls.hook.Process(p)
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
	if err := ls.gzWriter.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}

	// Get the compressed data.
	compressed := make([]byte, ls.buffer.Len())
	copy(compressed, ls.buffer.Bytes())

	// Enforce size cap.
	if len(compressed) > maxChunkSize {
		// Drop the oversize payload to preserve forward progress but report an error.
		// Reset state so subsequent writes proceed with a fresh chunk.
		ls.buffer.Reset()
		ls.gzWriter = gzip.NewWriter(&ls.buffer)
		return fmt.Errorf("compressed chunk exceeds 1 MiB: %d bytes", len(compressed))
	}

	// Reset buffer and gzip writer for next chunk.
	ls.buffer.Reset()
	ls.gzWriter = gzip.NewWriter(&ls.buffer)

	// Increment chunk number.
	currentChunkNo := ls.chunkNo
	ls.chunkNo++

	// Send to server (release lock during network call).
	ls.mu.Unlock()
	err := ls.sendChunk(compressed, currentChunkNo)
	ls.mu.Lock()

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
	url := fmt.Sprintf("%s/v1/nodes/%s/logs", ls.cfg.ServerURL, nodeID.String())
	// Per-request timeout; no struct-stored context.
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := ls.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
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
func (ls *LogStreamer) Close() error {
	var closeErr error
	ls.closeOnce.Do(func() {
		// Stop the background flusher.
		close(ls.stopCh)
		<-ls.flushDone

		// Flush any remaining logs.
		ls.mu.Lock()
		defer ls.mu.Unlock()

		if ls.buffer.Len() > 0 {
			closeErr = ls.flushLocked()
		}

		// Close the gzip writer.
		if err := ls.gzWriter.Close(); err != nil && closeErr == nil {
			closeErr = fmt.Errorf("close gzip writer: %w", err)
		}
	})

	return closeErr
}

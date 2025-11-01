package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// maxChunkSize is the maximum size of a gzipped log chunk (1 MiB).
	maxChunkSize = 1 << 20
	// flushInterval is how often to flush buffered logs to the server.
	flushInterval = 2 * time.Second
)

// LogStreamer buffers logs and streams them as gzipped chunks to the server.
type LogStreamer struct {
	cfg       Config
	runID     string
	stageID   string
	chunkNo   int32
	buffer    bytes.Buffer
	gzWriter  *gzip.Writer
	mu        sync.Mutex
	flushDone chan struct{}
	closeOnce sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewLogStreamer creates a new log streamer for a specific run.
func NewLogStreamer(cfg Config, runID string, stageID string) *LogStreamer {
	ctx, cancel := context.WithCancel(context.Background())
	ls := &LogStreamer{
		cfg:       cfg,
		runID:     runID,
		stageID:   stageID,
		chunkNo:   0,
		flushDone: make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
	ls.gzWriter = gzip.NewWriter(&ls.buffer)

	// Start background flusher.
	go ls.periodicFlush()

	return ls
}

// Write implements io.Writer interface for capturing logs.
func (ls *LogStreamer) Write(p []byte) (n int, err error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Write to gzip writer.
	written, err := ls.gzWriter.Write(p)
	if err != nil {
		return written, fmt.Errorf("write to gzip: %w", err)
	}

	// Check if we need to flush due to size.
	if err := ls.gzWriter.Flush(); err != nil {
		return written, fmt.Errorf("flush gzip: %w", err)
	}

	if ls.buffer.Len() >= maxChunkSize {
		if flushErr := ls.flushLocked(); flushErr != nil {
			slog.Warn("log streamer flush failed", "run_id", ls.runID, "error", flushErr)
		}
	}

	return written, nil
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
		case <-ls.ctx.Done():
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

	// Parse node ID from config.
	nodeID, err := uuid.Parse(ls.cfg.NodeID)
	if err != nil {
		return fmt.Errorf("parse node id: %w", err)
	}

	// Prepare request payload.
	payload := struct {
		RunID   string  `json:"run_id"`
		StageID *string `json:"stage_id,omitempty"`
		ChunkNo int32   `json:"chunk_no"`
		Data    []byte  `json:"data"`
	}{
		RunID:   ls.runID,
		ChunkNo: chunkNo,
		Data:    data,
	}
	if ls.stageID != "" {
		payload.StageID = &ls.stageID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// Send to server endpoint.
	url := fmt.Sprintf("%s/v1/nodes/%s/logs", ls.cfg.ServerURL, nodeID.String())
	req, err := http.NewRequestWithContext(ls.ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
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
		ls.cancel()
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

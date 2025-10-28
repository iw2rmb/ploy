package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/node/logstream"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// handleJobLogs dispatches log-related subpaths for a job.
func (s *controlPlaneServer) handleJobLogs(w http.ResponseWriter, r *http.Request, jobID string, parts []string) {
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	switch parts[0] {
	case "stream":
		s.handleJobLogsStream(w, r, jobID)
	case "snapshot":
		s.handleJobLogsSnapshot(w, r, jobID)
	case "entries":
		s.handleJobLogsEntry(w, r, jobID)
	default:
		http.NotFound(w, r)
	}
}

// handleJobLogsStream streams live job logs over Server-Sent Events.
func (s *controlPlaneServer) handleJobLogsStream(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}

	lastID, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}
	if err := logstream.Serve(w, r, s.streams, jobID, lastID); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
}

// handleJobLogsSnapshot returns a JSON snapshot of buffered log events.
func (s *controlPlaneServer) handleJobLogsSnapshot(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	events, err := s.snapshotLogStream(r.Context(), jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	dto := buildLogEventDTOs(events)
	writeJSON(w, http.StatusOK, map[string]any{"events": dto})
}

// handleJobLogsEntry appends log records emitted by workers.
func (s *controlPlaneServer) handleJobLogsEntry(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	if s.scheduler == nil {
		http.Error(w, "scheduler unavailable", http.StatusServiceUnavailable)
		return
	}
	defer r.Body.Close()

	var req struct {
		Ticket    string `json:"ticket"`
		NodeID    string `json:"node_id"`
		Stream    string `json:"stream"`
		Line      string `json:"line"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	req.Ticket = strings.TrimSpace(req.Ticket)
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.Stream = strings.TrimSpace(req.Stream)
	if req.Stream == "" {
		req.Stream = "stdout"
	}
	if req.Ticket == "" || req.NodeID == "" {
		http.Error(w, "ticket and node_id required", http.StatusBadRequest)
		return
	}

	job, err := s.scheduler.GetJob(r.Context(), req.Ticket, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if claimed := strings.TrimSpace(job.ClaimedBy); claimed != "" && !strings.EqualFold(claimed, req.NodeID) {
		http.Error(w, "node mismatch", http.StatusConflict)
		return
	}

	timestamp := strings.TrimSpace(req.Timestamp)
	if timestamp != "" {
		if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
			http.Error(w, "invalid timestamp", http.StatusBadRequest)
			return
		}
	} else {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	record := logstream.LogRecord{
		Timestamp: timestamp,
		Stream:    req.Stream,
		Line:      req.Line,
	}
	if err := s.streams.PublishLog(r.Context(), jobID, record); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleJobEvents streams job state changes via Server-Sent Events.
func (s *controlPlaneServer) handleJobEvents(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.etcd == nil {
		http.Error(w, "etcd unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	lastID, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}
	ticket, key, rev, err := s.lookupJobKey(r.Context(), jobID)
	if err != nil {
		writeErrorMessage(w, http.StatusNotFound, "job not found")
		return
	}
	job, err := s.scheduler.GetJob(r.Context(), ticket, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if _, err := io.WriteString(w, ":ok\n\n"); err != nil {
		return
	}
	flusher.Flush()

	if rev > 0 && (lastID <= 0 || rev > lastID) {
		if err := writeSSEJSON(w, rev, "job", jobDTOFrom(job)); err != nil {
			return
		}
		flusher.Flush()
	}

	watchRev := rev + 1
	if lastID >= watchRev {
		watchRev = lastID + 1
	}

	opts := []clientv3.OpOption{}
	if watchRev > 0 {
		opts = append(opts, clientv3.WithRev(watchRev))
	}
	watch := s.etcd.Watch(r.Context(), key, opts...)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case resp, ok := <-watch:
			if !ok {
				return
			}
			if err := resp.Err(); err != nil {
				continue
			}
			for _, ev := range resp.Events {
				if ev == nil || ev.Type != clientv3.EventTypePut || ev.Kv == nil {
					continue
				}
				current, err := s.scheduler.GetJob(r.Context(), ticket, jobID)
				if err != nil {
					continue
				}
				if err := writeSSEJSON(w, ev.Kv.ModRevision, "job", jobDTOFrom(current)); err != nil {
					return
				}
				flusher.Flush()
			}
		case <-ticker.C:
			if _, err := io.WriteString(w, ":ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// lookupJobKey finds the etcd key for a job ID and returns ticket, key, and revision.
func (s *controlPlaneServer) lookupJobKey(ctx context.Context, jobID string) (string, string, int64, error) {
	if s.etcd == nil {
		return "", "", 0, fmt.Errorf("etcd unavailable")
	}
	prefix := s.modsPrefix()
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	resp, err := s.etcd.Get(ctx, prefix, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		return "", "", 0, err
	}
	suffix := "/jobs/" + jobID
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		if !strings.HasSuffix(key, suffix) {
			continue
		}
		trimmed := strings.TrimPrefix(key, prefix)
		parts := strings.Split(trimmed, "/")
		if len(parts) < 3 {
			continue
		}
		ticket := strings.TrimSpace(parts[0])
		if ticket == "" {
			continue
		}
		detail, err := s.etcd.Get(ctx, key)
		if err != nil {
			return "", "", 0, err
		}
		if len(detail.Kvs) == 0 {
			continue
		}
		return ticket, key, detail.Kvs[0].ModRevision, nil
	}
	return "", "", 0, fmt.Errorf("job %s not found", jobID)
}

// modsPrefix returns the etcd prefix for Mods data, defaulting to mods/.
func (s *controlPlaneServer) modsPrefix() string {
	if s.mods != nil {
		if prefix := strings.TrimSpace(s.mods.Prefix()); prefix != "" {
			return prefix
		}
	}
	return "mods/"
}

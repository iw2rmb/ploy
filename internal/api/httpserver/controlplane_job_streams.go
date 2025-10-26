package httpserver

import (
	"context"
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

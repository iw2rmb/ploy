package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/node/logstream"
)

// Server exposes node-local streaming endpoints.
type Server struct {
	streams *logstream.Hub
}

// New constructs a node HTTP handler rooted at /node/v2.
func New(streams *logstream.Hub) http.Handler {
	mux := http.NewServeMux()
	srv := &Server{streams: streams}
	mux.HandleFunc("/node/v2/jobs/", srv.handleJobSubpath)
	return mux
}

func (s *Server) handleJobSubpath(w http.ResponseWriter, r *http.Request) {
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/node/v2/jobs/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) < 3 || parts[1] != "logs" || parts[2] != "stream" {
		http.NotFound(w, r)
		return
	}
	jobID := parts[0]
	if strings.TrimSpace(jobID) == "" {
		http.Error(w, "job id required", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var sinceID int64
	if raw := strings.TrimSpace(r.Header.Get("Last-Event-ID")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
			return
		}
		if id > 0 {
			sinceID = id
		}
	}

	if err := logstream.Serve(w, r, s.streams, jobID, sinceID); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
}

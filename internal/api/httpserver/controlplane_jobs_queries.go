package httpserver

import (
	"net/http"
	"strings"
)

// handleJobList returns all jobs bound to a ticket.
func (s *controlPlaneServer) handleJobList(w http.ResponseWriter, r *http.Request) {
	if !s.ensureScheduler(w) {
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		http.Error(w, "ticket query parameter required", http.StatusBadRequest)
		return
	}
	jobs, err := s.scheduler.ListJobs(r.Context(), ticket)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]jobDTO, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, jobDTOFrom(job))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": items})
}

// handleJobGet returns a specific job scoped by ticket and ID.
func (s *controlPlaneServer) handleJobGet(w http.ResponseWriter, r *http.Request, jobID string) {
	if !s.ensureScheduler(w) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		http.Error(w, "ticket query parameter required", http.StatusBadRequest)
		return
	}
	job, err := s.scheduler.GetJob(r.Context(), ticket, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, jobDTOFrom(job))
}

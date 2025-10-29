package httpserver

import (
	"net/http"
	"strings"
)

// handleModsSubpath dispatches /v1/mods/<ticket>/<action> to the correct handler.
func (s *controlPlaneServer) handleModsSubpath(w http.ResponseWriter, r *http.Request) {
	if !s.ensureMods(w) {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/mods/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(trimmed, "/")
	ticketID := strings.TrimSpace(parts[0])
	if ticketID == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		s.handleModsTicketStatus(w, r, ticketID)
		return
	}
	switch parts[1] {
	case "resume":
		s.handleModsResume(w, r, ticketID)
	case "cancel":
		s.handleModsCancel(w, r, ticketID)
	case "logs":
		s.handleModsLogs(w, r, ticketID, parts[2:])
	case "events":
		s.handleModsEvents(w, r, ticketID)
	default:
		if parts[1] == "hydration" {
			s.handleModsHydration(w, r, ticketID)
			return
		}
		http.NotFound(w, r)
	}
}

// handleModsTickets handles collection-level operations under /v1/mods/tickets.
func (s *controlPlaneServer) handleModsTickets(w http.ResponseWriter, r *http.Request) {
	if !s.ensureMods(w) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleModsSubmit(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleModsTicketSubpath handles /v1/mods/tickets/<ticket>/<action> routes.
func (s *controlPlaneServer) handleModsTicketSubpath(w http.ResponseWriter, r *http.Request) {
	if !s.ensureMods(w) {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/mods/tickets/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(trimmed, "/")
	ticketID := strings.TrimSpace(parts[0])
	if ticketID == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		s.handleModsTicketStatus(w, r, ticketID)
		return
	}
	switch parts[1] {
	case "cancel":
		s.handleModsCancel(w, r, ticketID)
	case "resume":
		s.handleModsResume(w, r, ticketID)
	case "logs":
		s.handleModsLogs(w, r, ticketID, parts[2:])
	case "events":
		s.handleModsEvents(w, r, ticketID)
	default:
		if parts[1] == "hydration" {
			s.handleModsHydration(w, r, ticketID)
			return
		}
		http.NotFound(w, r)
	}
}

// handleModsLogs dispatches log subpaths (snapshot or stream) for a ticket.
func (s *controlPlaneServer) handleModsLogs(w http.ResponseWriter, r *http.Request, ticketID string, parts []string) {
	if len(parts) == 0 {
		s.handleModsLogsSnapshot(w, r, ticketID)
		return
	}
	if len(parts) == 1 && parts[0] == "stream" {
		s.handleModsLogsStream(w, r, ticketID)
		return
	}
	http.NotFound(w, r)
}

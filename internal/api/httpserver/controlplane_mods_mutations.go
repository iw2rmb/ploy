package httpserver

import (
	"net/http"
	"strings"

	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// handleModsSubmit validates and submits a MODS ticket request.
func (s *controlPlaneServer) handleModsSubmit(w http.ResponseWriter, r *http.Request) {
	var req modsapi.TicketSubmitRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.TicketID) == "" {
		writeErrorMessage(w, http.StatusBadRequest, "ticket_id is required")
		return
	}
	if len(req.Stages) == 0 {
		writeErrorMessage(w, http.StatusBadRequest, "stages are required")
		return
	}
	spec := controlplanemods.TicketSpec{
		TicketID:   strings.TrimSpace(req.TicketID),
		Tenant:     strings.TrimSpace(req.Tenant),
		Submitter:  strings.TrimSpace(req.Submitter),
		Repository: strings.TrimSpace(req.Repository),
		Metadata:   cloneStringMap(req.Metadata),
		Stages:     make([]controlplanemods.StageDefinition, 0, len(req.Stages)),
	}
	for _, stage := range req.Stages {
		converted := controlplanemods.StageDefinition{
			ID:           strings.TrimSpace(stage.ID),
			Dependencies: cloneStringSlice(stage.Dependencies),
			Lane:         strings.TrimSpace(stage.Lane),
			Priority:     strings.TrimSpace(stage.Priority),
			MaxAttempts:  stage.MaxAttempts,
			Metadata:     cloneStringMap(stage.Metadata),
		}
		if converted.ID == "" {
			writeErrorMessage(w, http.StatusBadRequest, "stage id is required")
			return
		}
		spec.Stages = append(spec.Stages, converted)
	}
	status, err := s.mods.Submit(r.Context(), spec)
	if err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}
	resp := modsapi.TicketSubmitResponse{Ticket: toAPITicketSummary(status)}
	writeJSON(w, http.StatusAccepted, resp)
}

// handleModsTicketStatus returns the summary for a ticket.
func (s *controlPlaneServer) handleModsTicketStatus(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.mods.TicketStatus(r.Context(), ticketID)
	if err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}
	writeJSON(w, http.StatusOK, modsapi.TicketStatusResponse{Ticket: toAPITicketSummary(status)})
}

// handleModsCancel cancels a ticket when POST /cancel is called.
func (s *controlPlaneServer) handleModsCancel(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.mods.Cancel(r.Context(), ticketID); err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleModsResume resumes a paused ticket when POST /resume is called.
func (s *controlPlaneServer) handleModsResume(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.mods.Resume(r.Context(), ticketID)
	if err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}
	writeJSON(w, http.StatusOK, modsapi.TicketStatusResponse{Ticket: toAPITicketSummary(status)})
}

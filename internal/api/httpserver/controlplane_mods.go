package httpserver

import (
	"context"
	"errors"
	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	"io"
	"net/http"
	"strings"
	"time"
)

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
		http.NotFound(w, r)
	}
}

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
		http.NotFound(w, r)
	}
}

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

func (s *controlPlaneServer) handleModsLogsSnapshot(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	events, err := s.snapshotLogStream(r.Context(), modsLogStreamID(ticketID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	payload := map[string]any{
		"events": buildLogEventDTOs(events),
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *controlPlaneServer) handleModsLogsStream(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.streams == nil {
		http.Error(w, "log streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	since, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}
	if err := logstream.Serve(w, r, s.streams, modsLogStreamID(ticketID), since); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
}

func (s *controlPlaneServer) handleModsEvents(w http.ResponseWriter, r *http.Request, ticketID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mods == nil {
		http.Error(w, "mods service unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	since, err := parseLastEventID(r)
	if err != nil {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}

	status, rev, err := s.mods.TicketStatusWithRevision(r.Context(), ticketID)
	if err != nil {
		code, msg := mapModsError(err)
		writeErrorMessage(w, code, msg)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if _, err := io.WriteString(w, ":ok\n\n"); err != nil {
		return
	}
	flusher.Flush()

	if rev > 0 && (since <= 0 || rev > since) {
		if err := writeSSEJSON(w, rev, "ticket", toAPITicketSummary(status)); err != nil {
			return
		}
		flusher.Flush()
		since = rev
	}

	events, err := s.mods.WatchTicket(r.Context(), ticketID, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			switch evt.Kind {
			case controlplanemods.EventTicket:
				if evt.Ticket == nil {
					continue
				}
				if err := writeSSEJSON(w, evt.Revision, "ticket", toAPITicketSummary(evt.Ticket)); err != nil {
					return
				}
				flusher.Flush()
			case controlplanemods.EventStage:
				if evt.Stage == nil {
					continue
				}
				payload := modsStageEvent{
					TicketID: ticketID,
					Stage:    toAPIStageStatus(evt.Stage),
				}
				if err := writeSSEJSON(w, evt.Revision, "stage", payload); err != nil {
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

func mapModsError(err error) (int, string) {
	switch {
	case errors.Is(err, controlplanemods.ErrTicketNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, controlplanemods.ErrStageNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, controlplanemods.ErrStageAlreadyClaimed):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}

func toAPITicketSummary(status *controlplanemods.TicketStatus) modsapi.TicketSummary {
	if status == nil {
		return modsapi.TicketSummary{}
	}
	stages := make(map[string]modsapi.StageStatus, len(status.Stages))
	for id, stage := range status.Stages {
		stageCopy := stage
		stages[id] = toAPIStageStatus(&stageCopy)
	}
	return modsapi.TicketSummary{
		TicketID:   status.TicketID,
		State:      modsapi.TicketState(status.State),
		Tenant:     status.Tenant,
		Submitter:  status.Submitter,
		Repository: status.Repository,
		Metadata:   cloneStringMap(status.Metadata),
		CreatedAt:  status.CreatedAt.UTC(),
		UpdatedAt:  status.UpdatedAt.UTC(),
		Stages:     stages,
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

type modsStageEvent struct {
	TicketID string              `json:"ticket_id"`
	Stage    modsapi.StageStatus `json:"stage"`
}

func toAPIStageStatus(stage *controlplanemods.StageStatus) modsapi.StageStatus {
	if stage == nil {
		return modsapi.StageStatus{}
	}
	return modsapi.StageStatus{
		StageID:      stage.StageID,
		State:        modsapi.StageState(stage.State),
		Attempts:     stage.Attempts,
		MaxAttempts:  stage.MaxAttempts,
		CurrentJobID: stage.CurrentJobID,
		Artifacts:    cloneStringMap(stage.Artifacts),
		LastError:    stage.LastError,
	}
}

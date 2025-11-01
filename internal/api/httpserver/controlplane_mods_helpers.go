package httpserver

import (
	"errors"
	"net/http"

	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// mapModsError converts control-plane MODS errors into HTTP responses.
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

// toAPITicketSummary converts the internal ticket status into the API DTO.
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
		Submitter:  status.Submitter,
		Repository: status.Repository,
		Metadata:   cloneStringMap(status.Metadata),
		CreatedAt:  status.CreatedAt.UTC(),
		UpdatedAt:  status.UpdatedAt.UTC(),
		Stages:     stages,
	}
}

// cloneStringMap copies the provided string map to avoid aliasing request data.
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

// cloneStringSlice copies the provided slice to prevent shared references.
func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// modsStageEvent wraps stage updates for SSE payloads.
type modsStageEvent struct {
	TicketID string              `json:"ticket_id"`
	Stage    modsapi.StageStatus `json:"stage"`
}

// toAPIStageStatus converts an internal stage status into the API shape.
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

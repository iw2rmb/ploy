package httpserver

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// RepoDTO represents a repository in API responses.
type RepoDTO struct {
	ID        string  `json:"id"`
	URL       string  `json:"url"`
	Branch    *string `json:"branch,omitempty"`
	CommitSha *string `json:"commit_sha,omitempty"`
	CreatedAt string  `json:"created_at"`
}

// ModDTO represents a mod in API responses.
type ModDTO struct {
	ID        string          `json:"id"`
	RepoID    string          `json:"repo_id"`
	Spec      json.RawMessage `json:"spec"`
	CreatedBy *string         `json:"created_by,omitempty"`
	CreatedAt string          `json:"created_at"`
}

// RunDTO represents a run in API responses.
type RunDTO struct {
	ID         string          `json:"id"`
	ModID      string          `json:"mod_id"`
	Status     string          `json:"status"`
	Reason     *string         `json:"reason,omitempty"`
	CreatedAt  string          `json:"created_at"`
	StartedAt  string          `json:"started_at,omitempty"`
	FinishedAt string          `json:"finished_at,omitempty"`
	NodeID     string          `json:"node_id,omitempty"`
	BaseRef    string          `json:"base_ref"`
	TargetRef  string          `json:"target_ref"`
	CommitSha  *string         `json:"commit_sha,omitempty"`
	Stats      json.RawMessage `json:"stats,omitempty"`
}

// EventDTO represents an event in API responses (for SSE).
type EventDTO struct {
	ID      int64           `json:"id"`
	RunID   string          `json:"run_id"`
	StageID string          `json:"stage_id,omitempty"`
	Time    string          `json:"time"`
	Level   string          `json:"level"`
	Message string          `json:"message"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

// StageDTO represents a stage in API responses.
type StageDTO struct {
	ID         string          `json:"id"`
	RunID      string          `json:"run_id"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	StartedAt  string          `json:"started_at,omitempty"`
	FinishedAt string          `json:"finished_at,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	Meta       json.RawMessage `json:"meta,omitempty"`
}

// CreateRepoRequest represents a request to create a repository.
type CreateRepoRequest struct {
	URL       string  `json:"url"`
	Branch    *string `json:"branch,omitempty"`
	CommitSha *string `json:"commit_sha,omitempty"`
}

// CreateModRequest represents a request to create a mod.
type CreateModRequest struct {
	RepoID    string          `json:"repo_id"`
	Spec      json.RawMessage `json:"spec"`
	CreatedBy *string         `json:"created_by,omitempty"`
}

// CreateRunRequest represents a request to create a run.
type CreateRunRequest struct {
	ModID     string  `json:"mod_id"`
	BaseRef   string  `json:"base_ref"`
	TargetRef string  `json:"target_ref"`
	CommitSha *string `json:"commit_sha,omitempty"`
}

// CreateRunResponse represents the response after creating a run.
type CreateRunResponse struct {
	RunID string `json:"run_id"`
	Run   RunDTO `json:"run"`
}

// ListReposResponse represents a list of repositories.
type ListReposResponse struct {
	Repos []RepoDTO `json:"repos"`
}

// ListModsResponse represents a list of mods.
type ListModsResponse struct {
	Mods []ModDTO `json:"mods"`
}

// ListRunsResponse represents a list of runs.
type ListRunsResponse struct {
	Runs []RunDTO `json:"runs"`
}

// RunTimingDTO represents timing information for a run.
type RunTimingDTO struct {
	ID      string `json:"id"`
	QueueMs int64  `json:"queue_ms"`
	RunMs   int64  `json:"run_ms"`
}

// ListRunsTimingsResponse represents a list of run timings.
type ListRunsTimingsResponse struct {
	Timings []RunTimingDTO `json:"timings"`
}

// repoDTOFrom converts a store.Repo to a RepoDTO.
func repoDTOFrom(repo store.Repo) RepoDTO {
	return RepoDTO{
		ID:        uuidToString(repo.ID),
		URL:       repo.Url,
		Branch:    repo.Branch,
		CommitSha: repo.CommitSha,
		CreatedAt: timestampToString(repo.CreatedAt),
	}
}

// modDTOFrom converts a store.Mod to a ModDTO.
func modDTOFrom(mod store.Mod) ModDTO {
	spec := json.RawMessage("{}")
	if len(mod.Spec) > 0 {
		spec = mod.Spec
	}
	return ModDTO{
		ID:        uuidToString(mod.ID),
		RepoID:    uuidToString(mod.RepoID),
		Spec:      spec,
		CreatedBy: mod.CreatedBy,
		CreatedAt: timestampToString(mod.CreatedAt),
	}
}

// runDTOFrom converts a store.Run to a RunDTO.
func runDTOFrom(run store.Run) RunDTO {
	stats := json.RawMessage("{}")
	if len(run.Stats) > 0 {
		stats = run.Stats
	}
	dto := RunDTO{
		ID:        uuidToString(run.ID),
		ModID:     uuidToString(run.ModID),
		Status:    string(run.Status),
		Reason:    run.Reason,
		CreatedAt: timestampToString(run.CreatedAt),
		BaseRef:   run.BaseRef,
		TargetRef: run.TargetRef,
		CommitSha: run.CommitSha,
		Stats:     stats,
	}
	if run.StartedAt.Valid {
		dto.StartedAt = timestampToString(run.StartedAt)
	}
	if run.FinishedAt.Valid {
		dto.FinishedAt = timestampToString(run.FinishedAt)
	}
	if run.NodeID.Valid {
		dto.NodeID = uuidToString(run.NodeID)
	}
	return dto
}

// eventDTOFrom converts a store.Event to an EventDTO.
func eventDTOFrom(event store.Event) EventDTO {
	meta := json.RawMessage("{}")
	if len(event.Meta) > 0 {
		meta = event.Meta
	}
	dto := EventDTO{
		ID:      event.ID,
		RunID:   uuidToString(event.RunID),
		Time:    timestampToString(event.Time),
		Level:   event.Level,
		Message: event.Message,
		Meta:    meta,
	}
	if event.StageID.Valid {
		dto.StageID = uuidToString(event.StageID)
	}
	return dto
}

// stageDTOFrom converts a store.Stage to a StageDTO.
func stageDTOFrom(stage store.Stage) StageDTO {
	meta := json.RawMessage("{}")
	if len(stage.Meta) > 0 {
		meta = stage.Meta
	}
	dto := StageDTO{
		ID:         uuidToString(stage.ID),
		RunID:      uuidToString(stage.RunID),
		Name:       stage.Name,
		Status:     string(stage.Status),
		DurationMs: stage.DurationMs,
		Meta:       meta,
	}
	if stage.StartedAt.Valid {
		dto.StartedAt = timestampToString(stage.StartedAt)
	}
	if stage.FinishedAt.Valid {
		dto.FinishedAt = timestampToString(stage.FinishedAt)
	}
	return dto
}

// Helper functions for conversion.

func uuidToString(uuid pgtype.UUID) string {
	if !uuid.Valid {
		return ""
	}
	// Format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	b := uuid.Bytes
	return sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16],
	)
}

func sprintf(format string, parts ...[]byte) string {
	var result string
	formatParts := []interface{}{}
	for _, p := range parts {
		var val uint64
		for _, b := range p {
			val = val<<8 | uint64(b)
		}
		formatParts = append(formatParts, val)
	}
	// This is a simplified sprintf - in practice we'd use encoding/hex
	// But to avoid complexity, let's use the standard library properly
	if len(parts) == 5 {
		return hexUUID(parts[0], parts[1], parts[2], parts[3], parts[4])
	}
	return result
}

func hexUUID(a, b, c, d, e []byte) string {
	// Convert byte slices to hex strings and format as UUID
	return hexBytes(a) + "-" + hexBytes(b) + "-" + hexBytes(c) + "-" + hexBytes(d) + "-" + hexBytes(e)
}

func hexBytes(b []byte) string {
	const hex = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hex[v>>4]
		result[i*2+1] = hex[v&0x0f]
	}
	return string(result)
}

func timestampToString(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.Format(time.RFC3339Nano)
}

// runTimingDTOFrom converts a store.RunsTiming to a RunTimingDTO.
func runTimingDTOFrom(timing store.RunsTiming) RunTimingDTO {
	return RunTimingDTO{
		ID:      uuidToString(timing.ID),
		QueueMs: timing.QueueMs,
		RunMs:   timing.RunMs,
	}
}

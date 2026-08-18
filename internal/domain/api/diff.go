package api

import (
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

type DiffListItem struct {
	ID        types.DiffID      `json:"id"`
	JobID     types.JobID       `json:"job_id"`
	CreatedAt time.Time         `json:"created_at"`
	Size      int               `json:"gzipped_size"`
	Summary   types.DiffSummary `json:"summary,omitempty"`
}

type DiffListResponse struct {
	Diffs []DiffListItem `json:"diffs"`
}

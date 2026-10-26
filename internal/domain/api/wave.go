package api

import domaintypes "github.com/iw2rmb/ploy/internal/domain/types"

type WaveSummary struct {
	ID         domaintypes.WaveID     `json:"id"`
	MigID      domaintypes.MigID      `json:"mig_id"`
	SpecID     domaintypes.SpecID     `json:"spec_id"`
	CreatedBy  *string                `json:"created_by,omitempty"`
	Status     domaintypes.WaveStatus `json:"status"`
	CreatedAt  string                 `json:"created_at"`
	StartedAt  *string                `json:"started_at,omitempty"`
	FinishedAt *string                `json:"finished_at,omitempty"`
	Counts     *domaintypes.RunCounts `json:"run_counts,omitempty"`
}

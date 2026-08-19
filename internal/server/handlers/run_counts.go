package handlers

import (
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func runCountsFromStatusRows(rows []store.CountRunsByWaveStatusRow) *domaintypes.RunCounts {
	counts := &domaintypes.RunCounts{}
	for _, row := range rows {
		counts.Total += row.Count
		switch row.Status {
		case domaintypes.RunStatusQueued:
			counts.Queued = row.Count
		case domaintypes.RunStatusRunning:
			counts.Running = row.Count
		case domaintypes.RunStatusSuccess:
			counts.Success = row.Count
		case domaintypes.RunStatusFail:
			counts.Fail = row.Count
		case domaintypes.RunStatusCancelled:
			counts.Cancelled = row.Count
		}
	}
	counts.DerivedStatus = lifecycle.DeriveWaveStatus(counts)
	return counts
}

package handlers

import (
	"encoding/json"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// jobStepIndex reads optional next_id metadata from jobs.meta.
// next_id is no longer a dedicated DB column.
func jobStepIndex(job store.Job) domaintypes.StepIndex {
	var payload struct {
		StepIndex *float64 `json:"next_id,omitempty"`
	}
	if len(job.Meta) == 0 {
		return 0
	}
	if err := json.Unmarshal(job.Meta, &payload); err != nil || payload.StepIndex == nil {
		return 0
	}
	return domaintypes.StepIndex(*payload.StepIndex)
}

// withStepIndexMeta returns metadata that includes next_id.
// When meta is empty/invalid, it creates a new object with just next_id.
func withStepIndexMeta(meta []byte, stepIndex domaintypes.StepIndex) []byte {
	if !stepIndex.Valid() {
		return meta
	}
	obj := map[string]any{}
	if len(meta) > 0 {
		if err := json.Unmarshal(meta, &obj); err != nil || obj == nil {
			obj = map[string]any{}
		}
	}
	obj["next_id"] = stepIndex.Float64()
	encoded, err := json.Marshal(obj)
	if err != nil {
		return meta
	}
	return encoded
}

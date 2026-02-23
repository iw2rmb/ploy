package handlers

import (
	"encoding/json"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// jobStepIndex reads optional step_index metadata from jobs.meta.
// step_index is no longer a dedicated DB column.
func jobStepIndex(job store.Job) domaintypes.StepIndex {
	var payload struct {
		StepIndex *float64 `json:"step_index,omitempty"`
	}
	if len(job.Meta) == 0 {
		return 0
	}
	if err := json.Unmarshal(job.Meta, &payload); err != nil || payload.StepIndex == nil {
		return 0
	}
	return domaintypes.StepIndex(*payload.StepIndex)
}

// withStepIndexMeta returns metadata that includes step_index.
// When meta is empty/invalid, it creates a new object with just step_index.
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
	obj["step_index"] = stepIndex.Float64()
	encoded, err := json.Marshal(obj)
	if err != nil {
		return meta
	}
	return encoded
}

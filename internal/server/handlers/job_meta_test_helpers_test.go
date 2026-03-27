package handlers

import "encoding/json"

// withNextIDMeta injects a "next_id" key into meta JSON. This is used to
// simulate sort-order in mock result sets (e.g., listJobsByRunRepoAttemptResult)
// where the handler derives repo status from the last job by next_id ordering.
// It does NOT affect job-chain routing — that uses the Job.NextID pointer field.
func withNextIDMeta(meta []byte, nextID float64) []byte {
	obj := map[string]any{}
	if len(meta) > 0 {
		if err := json.Unmarshal(meta, &obj); err != nil || obj == nil {
			obj = map[string]any{}
		}
	}
	obj["next_id"] = nextID
	encoded, err := json.Marshal(obj)
	if err != nil {
		return meta
	}
	return encoded
}

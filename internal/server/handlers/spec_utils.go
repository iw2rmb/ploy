package handlers

import (
	"encoding/json"
	"strings"
)

// mergeStageIDIntoSpec merges stage_id into the JSON spec payload.
func mergeStageIDIntoSpec(spec json.RawMessage, stageID string) json.RawMessage {
	if strings.TrimSpace(stageID) == "" {
		return spec
	}
	var m map[string]any
	if len(spec) > 0 && json.Valid(spec) {
		_ = json.Unmarshal(spec, &m)
	}
	if m == nil {
		m = map[string]any{}
	}
	m["stage_id"] = stageID
	b, _ := json.Marshal(m)
	return json.RawMessage(b)
}

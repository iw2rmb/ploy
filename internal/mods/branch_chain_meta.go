package mods

import (
	"encoding/json"
	"fmt"
	"time"
)

// writeBranchChainStepMeta records chain metadata for a branch step and updates HEAD.
// It reads previous HEAD (if exists), writes steps/<stepID>/meta.json, and updates HEAD.json.
func writeBranchChainStepMeta(seaweed, execID, branchID, stepID, diffKey string) error {
	headKey := fmt.Sprintf("mods/%s/branches/%s/HEAD.json", execID, branchID)
	prevID := ""
	if b, code, _ := getJSONFn(seaweed, headKey); code == 200 {
		var head map[string]string
		_ = json.Unmarshal(b, &head)
		prevID = head["step_id"]
	}

	meta := map[string]any{
		"step_id":      stepID,
		"prev_step_id": prevID,
		"branch_id":    branchID,
		"diff_key":     diffKey,
		"ts":           time.Now().UTC().Format(time.RFC3339),
	}
	if mb, e := json.Marshal(meta); e == nil {
		if err := putJSONFn(seaweed, fmt.Sprintf("mods/%s/branches/%s/steps/%s/meta.json", execID, branchID, stepID), mb); err != nil {
			return err
		}
		if err := putJSONFn(seaweed, headKey, []byte(fmt.Sprintf("{\"step_id\":\"%s\"}", stepID))); err != nil {
			return err
		}
	}
	return nil
}

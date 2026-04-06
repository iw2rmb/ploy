package handlers

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func applyRerunAlterMutator(m map[string]any, in claimSpecMutatorInput) error {
	alter, ok, err := rerunAlterFromJobMeta(in.job.Meta)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	block, err := rerunSpecBlockForJobType(m, in.jobType)
	if err != nil {
		return err
	}
	if block == nil {
		return nil
	}

	if alter.Image != "" {
		block["image"] = alter.Image
	}
	if len(alter.Envs) > 0 {
		mergeEnvsOverrideBlock(block, alter.Envs)
	}
	if len(alter.In) > 0 {
		mergeRecordsByDstOverrideBlock(block, "in", alter.In)
	}
	return nil
}

func rerunAlterFromJobMeta(meta []byte) (rerunAlter, bool, error) {
	meta = []byte(strings.TrimSpace(string(meta)))
	if len(meta) == 0 {
		return rerunAlter{}, false, nil
	}
	var root map[string]any
	if err := json.Unmarshal(meta, &root); err != nil {
		return rerunAlter{}, false, fmt.Errorf("parse rerun metadata: %w", err)
	}
	raw, ok := root[rerunMetaKey]
	if !ok || raw == nil {
		return rerunAlter{}, false, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return rerunAlter{}, false, fmt.Errorf("%s must be an object", rerunMetaKey)
	}
	alterRaw, ok := obj[rerunMetaAlterKey].(map[string]any)
	if !ok {
		return rerunAlter{}, false, nil
	}
	alter, err := normalizeRerunAlter(alterRaw)
	if err != nil {
		return rerunAlter{}, false, err
	}
	return alter, true, nil
}

func rerunSpecBlockForJobType(m map[string]any, jobType domaintypes.JobType) (map[string]any, error) {
	switch jobType {
	case domaintypes.JobTypeHeal:
		bg, err := ensureObjectField(m, "build_gate", "spec")
		if err != nil {
			return nil, err
		}
		return ensureObjectField(bg, "heal", "spec.build_gate")
	case domaintypes.JobTypeReGate:
		return ensureObjectField(m, "re_gate", "spec")
	default:
		return nil, nil
	}
}

func mergeEnvsOverrideBlock(block map[string]any, overlay map[string]string) {
	if len(overlay) == 0 {
		return
	}
	existing := make(map[string]any)
	if raw, ok := block["envs"].(map[string]any); ok {
		for k, v := range raw {
			existing[k] = v
		}
	}
	keys := make([]string, 0, len(overlay))
	for k := range overlay {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		existing[k] = overlay[k]
	}
	block["envs"] = existing
}

func mergeRecordsByDstOverrideBlock(block map[string]any, field string, overlay []string) {
	if len(overlay) == 0 {
		return
	}

	indexByDst := map[string]int{}
	var merged []any
	if raw, ok := block[field].([]any); ok {
		for _, e := range raw {
			s, ok := e.(string)
			if !ok {
				merged = append(merged, e)
				continue
			}
			dst := hydraExtractDst(field, s)
			indexByDst[dst] = len(merged)
			merged = append(merged, s)
		}
	}

	for _, s := range overlay {
		dst := hydraExtractDst(field, s)
		if idx, ok := indexByDst[dst]; ok {
			merged[idx] = s
			continue
		}
		indexByDst[dst] = len(merged)
		merged = append(merged, s)
	}

	if len(merged) > 0 {
		block[field] = merged
	}
}

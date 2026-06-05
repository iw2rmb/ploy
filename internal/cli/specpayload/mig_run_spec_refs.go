package specpayload

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
)

type specRefStack struct {
	ids []string
}

func (s specRefStack) with(id string) (specRefStack, error) {
	for i, existing := range s.ids {
		if existing == id {
			cycle := append([]string{}, s.ids[i:]...)
			cycle = append(cycle, id)
			return specRefStack{}, fmt.Errorf("spec ref cycle detected: %s", strings.Join(cycle, " -> "))
		}
	}
	next := append([]string{}, s.ids...)
	next = append(next, id)
	return specRefStack{ids: next}, nil
}

func expandSpecRefsInPlace(spec map[string]any, sourcePath string) error {
	return expandSpecRefsInPlaceWithStack(spec, sourcePath, specRefStack{})
}

func expandSpecRefsInPlaceWithStack(spec map[string]any, sourcePath string, stack specRefStack) error {
	steps, ok := spec["steps"].([]any)
	if !ok {
		return nil
	}
	for i, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		refRaw, hasRef := step["ref"]
		if !hasRef {
			continue
		}
		if len(step) != 1 {
			return fmt.Errorf("steps[%d]: ref step must not contain other keys", i)
		}
		ref, ok := refRaw.(string)
		if !ok || strings.TrimSpace(ref) == "" {
			return fmt.Errorf("steps[%d].ref: required string", i)
		}
		imported, err := loadReferencedStep(ref, sourcePath, stack)
		if err != nil {
			return fmt.Errorf("steps[%d].ref: %w", i, err)
		}
		steps[i] = imported
	}
	return nil
}

func loadReferencedStep(rawRef, sourcePath string, stack specRefStack) (map[string]any, error) {
	refSpecPath, stepName, err := parseSpecStepRef(rawRef, sourcePath)
	if err != nil {
		return nil, err
	}

	id := refSpecPath + ":" + stepName
	nextStack, err := stack.with(id)
	if err != nil {
		return nil, err
	}

	data, err := common.ReadFileRooted(refSpecPath)
	if err != nil {
		return nil, fmt.Errorf("read referenced spec %s: %w", refSpecPath, err)
	}
	specBaseDir := filepath.Dir(refSpecPath)
	spec, err := parseSpecInputToMap(data, specBaseDir)
	if err != nil {
		return nil, fmt.Errorf("parse referenced spec %s: %w", refSpecPath, err)
	}
	if err := expandSpecRefsInPlaceWithStack(spec, refSpecPath, nextStack); err != nil {
		return nil, err
	}

	step, err := selectNamedStep(spec, stepName, refSpecPath)
	if err != nil {
		return nil, err
	}
	normalizeStepLocalPaths(step, refSpecPath)
	return step, nil
}

func parseSpecStepRef(rawRef, sourcePath string) (string, string, error) {
	ref := strings.TrimSpace(rawRef)
	idx := strings.LastIndex(ref, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("expected <spec-path>:<step-name>")
	}
	pathPart := strings.TrimSpace(ref[:idx])
	stepName := strings.TrimSpace(ref[idx+1:])
	if pathPart == "" {
		return "", "", fmt.Errorf("spec path is required")
	}
	if stepName == "" {
		return "", "", fmt.Errorf("step name is required")
	}

	specPath := pathPart
	if !filepath.IsAbs(specPath) {
		specPath = filepath.Join(filepath.Dir(sourcePath), specPath)
	}
	specPath = filepath.Clean(specPath)

	info, err := os.Stat(specPath)
	if err != nil {
		return "", "", fmt.Errorf("load referenced spec: %w", err)
	}
	if info.IsDir() {
		specPath = filepath.Join(specPath, "mig.yaml")
		if _, err := os.Stat(specPath); err != nil {
			return "", "", fmt.Errorf("load referenced spec: %w", err)
		}
	}
	absSpecPath, err := filepath.Abs(specPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve referenced spec %s: %w", specPath, err)
	}
	return absSpecPath, stepName, nil
}

func selectStepInPlace(spec map[string]any, stepSelector string, sourcePath string) error {
	selector := strings.TrimSpace(stepSelector)
	if selector == "" {
		return nil
	}
	step, err := selectNamedStep(spec, selector, sourcePath)
	if err != nil {
		return err
	}
	spec["steps"] = []any{step}
	return nil
}

func selectNamedStep(spec map[string]any, stepName string, sourcePath string) (map[string]any, error) {
	steps, ok := spec["steps"].([]any)
	if !ok {
		return nil, fmt.Errorf("%s: steps must be an array", sourcePath)
	}

	byName := make(map[string]map[string]any, len(steps))
	byNameIndex := make(map[string]int, len(steps))
	for i, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: steps[%d] must be an object", sourcePath, i)
		}
		rawName, ok := step["name"].(string)
		name := strings.TrimSpace(rawName)
		if !ok || name == "" {
			return nil, fmt.Errorf("%s: steps[%d].name is required for step selection", sourcePath, i)
		}
		if _, exists := byName[name]; exists {
			return nil, fmt.Errorf("%s: steps[%d].name duplicate %q (already used at steps[%d])", sourcePath, i, name, byNameIndex[name])
		}
		byName[name] = step
		byNameIndex[name] = i
	}

	step, ok := byName[stepName]
	if !ok {
		return nil, fmt.Errorf("%s: step %q not found", sourcePath, stepName)
	}
	return cloneSpecMap(step), nil
}

func cloneSpecMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneSpecValue(v)
	}
	return out
}

func cloneSpecValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		return cloneSpecMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = cloneSpecValue(typed[i])
		}
		return out
	default:
		return typed
	}
}

func normalizeStepLocalPaths(step map[string]any, sourcePath string) {
	normalizeStepMountList(step, "in", sourcePath, false)
	normalizeStepMountList(step, "out", sourcePath, false)
	normalizeStepMountList(step, "home", sourcePath, true)
}

func normalizeStepMountList(step map[string]any, key string, sourcePath string, isHome bool) {
	raw, ok := step[key].([]any)
	if !ok {
		return
	}
	for i, entryRaw := range raw {
		entry, ok := entryRaw.(string)
		if !ok {
			continue
		}
		raw[i] = normalizeMountEntrySource(entry, sourcePath, isHome)
	}
}

func normalizeMountEntrySource(entry string, sourcePath string, isHome bool) string {
	body := strings.TrimSpace(entry)
	if body == "" {
		return entry
	}
	suffix := ""
	if isHome && strings.HasSuffix(body, ":ro") {
		body = strings.TrimSuffix(body, ":ro")
		suffix = ":ro"
	}
	if idx := strings.Index(body, ":"); idx > 0 && shortHashPattern.MatchString(body[:idx]) {
		return entry
	}
	idx := strings.LastIndex(body, ":")
	if idx <= 0 {
		return entry
	}

	src := strings.TrimSpace(body[:idx])
	dst := body[idx:]
	return normalizeLocalSourcePath(sourcePath, src) + dst + suffix
}

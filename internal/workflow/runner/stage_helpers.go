package runner

import (
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
)

// resolvedStage merges execution output into the base stage configuration.
func resolvedStage(base Stage, outcome Stage) Stage {
	resolved := outcome
	if strings.TrimSpace(resolved.Name) == "" {
		return base
	}
	if strings.TrimSpace(resolved.Lane) == "" {
		resolved.Lane = base.Lane
	}
	if len(resolved.Dependencies) == 0 {
		resolved.Dependencies = copyStringSlice(base.Dependencies)
	}
	if strings.TrimSpace(resolved.CacheKey) == "" {
		resolved.CacheKey = base.CacheKey
	}
	if resolved.Constraints.Manifest.Manifest.Name == "" && resolved.Constraints.Manifest.Manifest.Version == "" {
		resolved.Constraints.Manifest = base.Constraints.Manifest
	}
	if !resolved.Aster.Enabled && base.Aster.Enabled {
		resolved.Aster = base.Aster
	} else {
		if resolved.Aster.Enabled {
			if len(resolved.Aster.Toggles) == 0 {
				resolved.Aster.Toggles = copyStringSlice(base.Aster.Toggles)
			}
			if len(resolved.Aster.Bundles) == 0 {
				resolved.Aster.Bundles = append([]aster.Metadata(nil), base.Aster.Bundles...)
			}
		}
	}
	resolved.Job = mergeStageJob(base.Job, resolved.Job)
	return resolved
}

func mergeStageJob(base, override StageJobSpec) StageJobSpec {
	merged := override
	if strings.TrimSpace(merged.Image) == "" {
		merged.Image = base.Image
	}
	if len(merged.Command) == 0 && len(base.Command) > 0 {
		merged.Command = append([]string(nil), base.Command...)
	}
	if len(merged.Env) == 0 && len(base.Env) > 0 {
		merged.Env = copyStringMap(base.Env)
	}
	if strings.TrimSpace(merged.Resources.CPU) == "" {
		merged.Resources.CPU = base.Resources.CPU
	}
	if strings.TrimSpace(merged.Resources.Memory) == "" {
		merged.Resources.Memory = base.Resources.Memory
	}
	if strings.TrimSpace(merged.Resources.Disk) == "" {
		merged.Resources.Disk = base.Resources.Disk
	}
	if strings.TrimSpace(merged.Resources.GPU) == "" {
		merged.Resources.GPU = base.Resources.GPU
	}
	if strings.TrimSpace(merged.Runtime) == "" {
		merged.Runtime = base.Runtime
	}
	merged.Metadata = mergeStringMaps(base.Metadata, merged.Metadata)
	return merged
}

// copyStringSlice duplicates a slice while trimming whitespace and omitting blanks.
func copyStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		if trimmedKey := strings.TrimSpace(key); trimmedKey != "" {
			dst[trimmedKey] = strings.TrimSpace(value)
		}
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func mergeStringMaps(base, override map[string]string) map[string]string {
	if len(base) == 0 {
		return copyStringMap(override)
	}
	result := copyStringMap(base)
	if result == nil {
		result = make(map[string]string)
	}
	for key, value := range override {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		result[trimmedKey] = strings.TrimSpace(value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

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
	return resolved
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

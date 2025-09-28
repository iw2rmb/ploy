package runner

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

// resolveStageAster builds the Aster metadata for a stage based on manifest toggles and overrides.
func resolveStageAster(ctx context.Context, stage Stage, manifest manifests.Compilation, opts AsterOptions) (StageAster, error) {
	if !opts.Enabled {
		return StageAster{}, nil
	}

	stageKey := strings.ToLower(strings.TrimSpace(stage.Name))
	override := opts.StageOverrides
	if override == nil {
		override = map[string]AsterStageOverride{}
	}
	config := override[stageKey]
	if config.Disable {
		return StageAster{}, nil
	}

	active := make([]string, 0, len(manifest.Aster.Required)+len(opts.AdditionalToggles)+len(config.ExtraToggles))
	active = append(active, manifest.Aster.Required...)
	active = append(active, opts.AdditionalToggles...)
	active = append(active, config.ExtraToggles...)
	toggles := normalizeAsterToggles(active)
	if len(toggles) == 0 {
		return StageAster{}, nil
	}
	if opts.Locator == nil {
		return StageAster{}, ErrAsterLocatorRequired
	}
	bundles := make([]aster.Metadata, 0, len(toggles))
	stageName := strings.TrimSpace(stage.Name)
	for _, toggle := range toggles {
		if err := ctx.Err(); err != nil {
			return StageAster{}, err
		}
		meta, err := opts.Locator.Locate(ctx, aster.Request{Stage: stageName, Toggle: toggle})
		if err != nil {
			return StageAster{}, fmt.Errorf("locate Aster bundle for stage %s toggle %s: %w", stageName, toggle, err)
		}
		if strings.TrimSpace(meta.Stage) == "" {
			meta.Stage = stageName
		}
		if strings.TrimSpace(meta.Toggle) == "" {
			meta.Toggle = toggle
		}
		bundles = append(bundles, meta)
	}
	return StageAster{Enabled: true, Toggles: toggles, Bundles: bundles}, nil
}

// normalizeAsterToggles cleans and sorts toggles while removing duplicates.
func normalizeAsterToggles(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if trimmed := strings.ToLower(strings.TrimSpace(value)); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

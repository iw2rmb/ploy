package mods

import (
	"os"
	"path/filepath"

	nomadtpl "github.com/iw2rmb/ploy/platform/nomad/mods"
)

// RenderORWApplyAssets writes a rendered orw_apply.hcl for the given option ID.
func (r *ModRunner) RenderORWApplyAssets(optionID string) (string, error) {
	dir := filepath.Join(r.workspaceDir, string(StepTypeORWApply), optionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	renderedPath := filepath.Join(dir, "orw_apply.rendered.hcl")
	if err := os.WriteFile(renderedPath, nomadtpl.GetORWApplyTemplate(), 0644); err != nil {
		return "", err
	}
	return renderedPath, nil
}

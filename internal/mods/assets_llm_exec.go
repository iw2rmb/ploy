package mods

import (
	"os"
	"path/filepath"

	nomadtpl "github.com/iw2rmb/ploy/platform/nomad/mods"
)

// RenderLLMExecAssets writes a rendered llm_exec.hcl for the given option ID.
func (r *ModRunner) RenderLLMExecAssets(optionID string) (string, error) {
	dir := filepath.Join(r.workspaceDir, string(StepTypeLLMExec), optionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	renderedPath := filepath.Join(dir, "llm_exec.rendered.hcl")
	// Defer env substitution to caller (same as planner/reducer), we just copy template here
	if err := os.WriteFile(renderedPath, nomadtpl.GetLLMExecTemplate(), 0644); err != nil {
		return "", err
	}
	return renderedPath, nil
}

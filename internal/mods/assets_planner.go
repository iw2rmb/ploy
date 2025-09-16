package mods

import (
	"fmt"
	"os"
	"path/filepath"

	nomadtpl "github.com/iw2rmb/ploy/platform/nomad/mods"
)

// PlannerAssets holds file paths for rendered planner inputs and HCL
type PlannerAssets struct {
	InputsPath string
	HCLPath    string
}

// RenderPlannerAssets writes minimal inputs.json and a rendered planner.hcl (with placeholders) into the workspace.
// This is a dry-run helper to prepare artifacts for planner submission later.
func (r *ModRunner) RenderPlannerAssets() (*PlannerAssets, error) {
	inputsDir := filepath.Join(r.workspaceDir, "planner", "context")
	outDir := filepath.Join(r.workspaceDir, "planner", "out")
	if err := os.MkdirAll(inputsDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, err
	}
	// Minimal inputs.json
	inputsPath := filepath.Join(inputsDir, "inputs.json")
	inputs := fmt.Sprintf(`{
  "language": "java",
  "lane": %q,
  "last_error": {"stdout": "", "stderr": ""},
  "deps": {}
}`, r.config.Lane)

	if err := os.WriteFile(inputsPath, []byte(inputs), 0644); err != nil {
		return nil, err
	}

	// Write embedded planner template into workspace
	hclPath := filepath.Join(r.workspaceDir, "planner", "planner.hcl")
	if err := os.WriteFile(hclPath, nomadtpl.GetPlannerTemplate(), 0644); err != nil {
		return nil, err
	}

	return &PlannerAssets{InputsPath: inputsPath, HCLPath: hclPath}, nil
}

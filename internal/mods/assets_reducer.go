package mods

import (
	"os"
	"path/filepath"

	nomadtpl "github.com/iw2rmb/ploy/platform/nomad/mods"
)

// ReducerAssets holds file paths for rendered reducer inputs and HCL
type ReducerAssets struct {
	HistoryPath string
	HCLPath     string
}

// RenderReducerAssets writes a minimal history.json and a rendered reducer.hcl (with placeholders) into the workspace.
func (r *ModRunner) RenderReducerAssets() (*ReducerAssets, error) {
	ctxDir := filepath.Join(r.workspaceDir, "reducer", "context")
	if err := os.MkdirAll(ctxDir, 0755); err != nil {
		return nil, err
	}

	// Minimal history.json
	historyPath := filepath.Join(ctxDir, "history.json")
	history := "{\n  \"plan_id\": \"\",\n  \"branches\": [],\n  \"winner\": \"\"\n}"
	if err := os.WriteFile(historyPath, []byte(history), 0644); err != nil {
		return nil, err
	}

	hclPath := filepath.Join(r.workspaceDir, "reducer", "reducer.hcl")
	if err := os.WriteFile(hclPath, nomadtpl.GetReducerTemplate(), 0644); err != nil {
		return nil, err
	}

	return &ReducerAssets{HistoryPath: historyPath, HCLPath: hclPath}, nil
}

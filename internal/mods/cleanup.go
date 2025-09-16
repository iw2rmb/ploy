package mods

import "os"

// CleanupWorkspace removes the temporary workspace directory
func (r *ModRunner) CleanupWorkspace() error {
	if r.workspaceDir != "" {
		return os.RemoveAll(r.workspaceDir)
	}
	return nil
}

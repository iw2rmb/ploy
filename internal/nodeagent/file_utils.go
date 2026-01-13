package nodeagent

import (
	"log/slog"
	"os"
	"path/filepath"
)

// listFilesRecursive returns whether directory has any files and a slice of absolute file paths.
// Walk errors (e.g., permission denied) are logged but don't stop the walk.
func listFilesRecursive(root string) (bool, []string) {
	var out []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Log walk errors for visibility but continue walking.
			// Permission errors are common and shouldn't stop the entire walk.
			slog.Warn("file walk error", "path", path, "error", err)
			return nil
		}
		if info == nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		out = append(out, path)
		return nil
	})
	return len(out) > 0, out
}

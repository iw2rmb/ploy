package nodeagent

import (
	"os"
	"path/filepath"
)

// listFilesRecursive returns whether directory has any files and a slice of absolute file paths.
func listFilesRecursive(root string) (bool, []string) {
	var out []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
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

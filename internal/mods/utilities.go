package mods

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// jsonUnmarshal provides lightweight JSON unmarshal to avoid adding deps
func jsonUnmarshal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

// findPlanJSON attempts to locate a plan.json inside the preserved workspace
// Returns first match path or empty string
func findPlanJSON(workspace string) string {
	if workspace == "" {
		return ""
	}
	var found string
	_ = filepath.WalkDir(workspace, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if !d.IsDir() && strings.EqualFold(filepath.Base(p), "plan.json") {
			found = p
			return io.EOF // stop walking
		}
		return nil
	})
	return found
}

// cloneRepo clones a repository at a specific ref into dest (shallow)
func cloneRepo(repoURL, ref, dest string) error {
	if repoURL == "" || dest == "" {
		return fmt.Errorf("missing repoURL or dest")
	}
	// Ensure parent exists
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	args := []string{"clone", "--depth", "1"}
	if ref != "" && ref != "main" && ref != "master" {
		args = append(args, "-b", ref)
	}
	args = append(args, repoURL, dest)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %v: %s", err, string(out))
	}
	return nil
}

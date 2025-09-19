package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// IsFileIgnored checks if a file is ignored by .gitignore.
func (g *GitUtils) IsFileIgnored(filePath string) (bool, error) {
	cmd := exec.Command("git", "check-ignore", filePath)
	cmd.Dir = g.workingDir

	err := cmd.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode() == 0, nil
		}
		return false, fmt.Errorf("failed to check if file is ignored: %w", err)
	}

	return true, nil
}

// GetIgnoredFiles returns a list of files that would be ignored by .gitignore.
func (g *GitUtils) GetIgnoredFiles() ([]string, error) {
	cmd := exec.Command("git", "ls-files", "--others", "--ignored", "--exclude-standard")
	cmd.Dir = g.workingDir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ignored files: %w", err)
	}

	var ignoredFiles []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if file := strings.TrimSpace(line); file != "" {
			ignoredFiles = append(ignoredFiles, file)
		}
	}

	return ignoredFiles, nil
}

package arf

import (
	"fmt"
	"os/exec"
)

// executeCommand executes a shell command
func executeCommand(cmd string) error {
	return executeCommandWithError(cmd)
}

// executeCommandWithOutput executes a shell command and captures output
func executeCommandWithOutput(cmd string) (string, error) {
	// Use shell to execute command and capture output
	cmdParts := []string{"sh", "-c", cmd}
	output, err := exec.Command(cmdParts[0], cmdParts[1:]...).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// executeCommandWithError executes a shell command and returns error if any
func executeCommandWithError(cmd string) error {
	// Use shell to execute command
	cmdParts := []string{"sh", "-c", cmd}
	return exec.Command(cmdParts[0], cmdParts[1:]...).Run()
}

// stringPtr returns a pointer to the given string
func stringPtr(s string) *string {
	return &s
}

// intPtr returns a pointer to the given integer
func intPtr(i int) *int {
	return &i
}

// generateDiff generates a git diff for the given repository path
func (d *OpenRewriteDispatcher) generateDiff(repoPath string) (string, error) {
	cmd := fmt.Sprintf("cd %s && git diff", repoPath)
	output, err := executeCommandWithOutput(cmd)
	if err != nil {
		return "", err
	}
	return output, nil
}

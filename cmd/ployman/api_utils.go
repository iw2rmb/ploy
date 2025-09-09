package main

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists without affecting it
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func getCurrentCommit() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

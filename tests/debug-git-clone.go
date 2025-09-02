package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	repoURL := "https://github.com/winterbe/java8-tutorial.git"
	branch := "master"
	targetPath := "/tmp/test-git-clone/api-simulation/test-repo"

	fmt.Printf("=== API Git Clone Simulation ===\n")
	fmt.Printf("URL: %s\n", repoURL)
	fmt.Printf("Branch: %s\n", branch)
	fmt.Printf("Target: %s\n", targetPath)
	fmt.Printf("Working Directory: %s\n", os.Getenv("PWD"))
	fmt.Printf("HOME: %s\n", os.Getenv("HOME"))
	fmt.Printf("PATH: %s\n", os.Getenv("PATH")[:100] + "...")
	fmt.Printf("\n")

	// Replicate the exact API logic
	args := []string{"clone", "--depth=1"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, repoURL, targetPath)

	fmt.Printf("Command: git %v\n", args)

	// Execute git clone
	cmd := exec.CommandContext(context.Background(), "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	fmt.Printf("Executing...\n")
	if err := cmd.Run(); err != nil {
		fmt.Printf("ERROR: git clone failed: %v - %s\n", err, stderr.String())
		os.Exit(1)
	}

	fmt.Printf("SUCCESS: Git clone completed\n\n")

	// Check results
	fileCount := 0
	err := filepath.Walk(targetPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			fileCount++
		}
		return nil
	})
	
	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
	}

	fmt.Printf("=== Results ===\n")
	fmt.Printf("Target directory exists: %v\n", dirExists(targetPath))
	fmt.Printf("File count: %d\n", fileCount)
	
	if fileCount > 0 {
		fmt.Printf("Sample files:\n")
		files, _ := filepath.Glob(filepath.Join(targetPath, "*"))
		for i, file := range files {
			if i >= 10 {
				break
			}
			info, _ := os.Stat(file)
			if info != nil {
				fmt.Printf("  %s (%v)\n", filepath.Base(file), info.IsDir())
			}
		}
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}
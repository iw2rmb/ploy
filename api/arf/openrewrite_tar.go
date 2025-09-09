package arf

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// createTarFromRepo creates a tar archive from a repository directory
func (d *OpenRewriteDispatcher) createTarFromRepo(repoPath, tarPath string) error {
	log.Printf("[OpenRewrite Dispatcher] ===== TAR CREATION START =====")
	log.Printf("[OpenRewrite Dispatcher] Source repo path: %s", repoPath)
	log.Printf("[OpenRewrite Dispatcher] Target tar path: %s", tarPath)

	// Validate repo path exists and analyze contents
	repoInfo, err := os.Stat(repoPath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Repository path does not exist: %s - %v", repoPath, err)
		return fmt.Errorf("repository path does not exist: %s", repoPath)
	}
	log.Printf("[OpenRewrite Dispatcher] Repository path exists: isDir=%v, size=%d", repoInfo.IsDir(), repoInfo.Size())

	// Count files in repository before tar creation
	fileCount := 0
	var totalSize int64
	err = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("[OpenRewrite Dispatcher] Warning: Error walking path %s: %v", path, err)
			return nil // Skip errors
		}
		if !info.IsDir() {
			fileCount++
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] Warning: Error analyzing repository: %v", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Repository analysis: %d files, %d bytes total", fileCount, totalSize)

	// List some sample files for debugging
	log.Printf("[OpenRewrite Dispatcher] Sample files in repository:")
	files, err := os.ReadDir(repoPath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] Warning: Cannot read directory: %v", err)
	} else {
		sampleCount := 0
		for _, file := range files {
			if sampleCount >= 10 {
				log.Printf("[OpenRewrite Dispatcher] ... and %d more files", len(files)-10)
				break
			}
			log.Printf("[OpenRewrite Dispatcher]   - %s (isDir: %v)", file.Name(), file.IsDir())
			sampleCount++
		}
	}

	// Remove existing tar file if it exists
	if _, err := os.Stat(tarPath); err == nil {
		log.Printf("[OpenRewrite Dispatcher] Removing existing tar file: %s", tarPath)
		if err := os.Remove(tarPath); err != nil {
			log.Printf("[OpenRewrite Dispatcher] Warning: Failed to remove existing tar file: %v", err)
		}
	}

	// Use tar command to create archive with comprehensive logging
	cmd := fmt.Sprintf("tar -cf %s -C %s .", tarPath, repoPath)
	log.Printf("[OpenRewrite Dispatcher] Executing tar command: %s", cmd)

	startTime := time.Now()
	if err := executeCommand(cmd); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Tar command failed after %v: %v", time.Since(startTime), err)
		log.Printf("[OpenRewrite Dispatcher] Command was: %s", cmd)
		return fmt.Errorf("failed to create tar archive: %w", err)
	}
	duration := time.Since(startTime)
	log.Printf("[OpenRewrite Dispatcher] Tar command completed successfully in %v", duration)

	// Verify tar file was created and analyze it
	fileInfo, err := os.Stat(tarPath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Tar file was not created: %s - %v", tarPath, err)
		return fmt.Errorf("tar file was not created: %w", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Tar file created successfully: %s", tarPath)
	log.Printf("[OpenRewrite Dispatcher] Tar file size: %d bytes", fileInfo.Size())

	// Test tar file integrity by listing contents
	log.Printf("[OpenRewrite Dispatcher] Verifying tar file integrity...")
	listCmd := fmt.Sprintf("tar -tf %s", tarPath)
	output, err := executeCommandWithOutput(listCmd)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] Warning: Cannot verify tar contents: %v", err)
	} else {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		log.Printf("[OpenRewrite Dispatcher] Tar contains %d entries", len(lines))
		// Show first few entries
		for i, line := range lines {
			if i >= 5 {
				log.Printf("[OpenRewrite Dispatcher] ... and %d more entries", len(lines)-5)
				break
			}
			log.Printf("[OpenRewrite Dispatcher]   - %s", line)
		}
	}

	log.Printf("[OpenRewrite Dispatcher] ===== TAR CREATION SUCCESS =====")
	return nil
}

// extractTarToRepo extracts a tar archive to a repository directory
func (d *OpenRewriteDispatcher) extractTarToRepo(tarPath, repoPath string) error {
	log.Printf("[OpenRewrite Dispatcher] ===== TAR EXTRACTION START =====")
	log.Printf("[OpenRewrite Dispatcher] Source tar path: %s", tarPath)
	log.Printf("[OpenRewrite Dispatcher] Target repo path: %s", repoPath)

	// Verify tar file exists and analyze it
	tarInfo, err := os.Stat(tarPath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Tar file does not exist: %s - %v", tarPath, err)
		return fmt.Errorf("tar file does not exist: %s", tarPath)
	}
	log.Printf("[OpenRewrite Dispatcher] Tar file exists: size=%d bytes", tarInfo.Size())

	// Verify tar file is not empty
	if tarInfo.Size() == 0 {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Tar file is empty: %s", tarPath)
		return fmt.Errorf("tar file is empty: %s", tarPath)
	}

	// Analyze tar contents before extraction
	log.Printf("[OpenRewrite Dispatcher] Analyzing tar contents...")
	listCmd := fmt.Sprintf("tar -tf %s", tarPath)
	output, err := executeCommandWithOutput(listCmd)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot list tar contents: %v", err)
		return fmt.Errorf("failed to list tar contents: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	log.Printf("[OpenRewrite Dispatcher] Tar contains %d entries", len(lines))

	// Show first few entries
	for i, line := range lines {
		if i >= 10 {
			log.Printf("[OpenRewrite Dispatcher] ... and %d more entries", len(lines)-10)
			break
		}
		log.Printf("[OpenRewrite Dispatcher]   - %s", line)
	}

	// Remove existing repo directory if it exists
	if _, err := os.Stat(repoPath); err == nil {
		log.Printf("[OpenRewrite Dispatcher] Removing existing repo directory: %s", repoPath)
		if err := os.RemoveAll(repoPath); err != nil {
			log.Printf("[OpenRewrite Dispatcher] Warning: Failed to remove existing repo directory: %v", err)
		}
	}

	// Create repo directory
	log.Printf("[OpenRewrite Dispatcher] Creating repo directory: %s", repoPath)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Failed to create repo directory: %v", err)
		return fmt.Errorf("failed to create repo directory: %w", err)
	}

	// Extract tar to repo directory
	cmd := fmt.Sprintf("tar -xf %s -C %s", tarPath, repoPath)
	log.Printf("[OpenRewrite Dispatcher] Executing extraction command: %s", cmd)

	startTime := time.Now()
	if err := executeCommand(cmd); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Tar extraction failed after %v: %v", time.Since(startTime), err)
		log.Printf("[OpenRewrite Dispatcher] Command was: %s", cmd)
		return fmt.Errorf("failed to extract tar archive: %w", err)
	}
	duration := time.Since(startTime)
	log.Printf("[OpenRewrite Dispatcher] Tar extraction completed successfully in %v", duration)

	// Verify extraction by checking directory contents
	log.Printf("[OpenRewrite Dispatcher] Verifying extraction...")
	extractedFiles, err := os.ReadDir(repoPath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot read extracted directory: %v", err)
		return fmt.Errorf("failed to read extracted directory: %w", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Extracted %d files/directories", len(extractedFiles))

	// Show sample of extracted files
	for i, file := range extractedFiles {
		if i >= 10 {
			log.Printf("[OpenRewrite Dispatcher] ... and %d more files/directories", len(extractedFiles)-10)
			break
		}
		log.Printf("[OpenRewrite Dispatcher]   - %s (isDir: %v)", file.Name(), file.IsDir())
	}

	log.Printf("[OpenRewrite Dispatcher] ===== TAR EXTRACTION SUCCESS =====")
	return nil
}

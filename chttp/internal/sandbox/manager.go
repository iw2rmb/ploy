package sandbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Manager handles secure execution of CLI tools in a sandboxed environment
type Manager struct {
	workDir   string
	runAsUser string
	maxMemory string
	maxCPU    string
}

// ExecutionResult contains the result of a command execution
type ExecutionResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// NewManager creates a new sandbox manager
func NewManager(workDir, runAsUser, maxMemory, maxCPU string) *Manager {
	return &Manager{
		workDir:   workDir,
		runAsUser: runAsUser,
		maxMemory: maxMemory,
		maxCPU:    maxCPU,
	}
}

// ExtractArchive extracts a gzipped tar archive to a temporary directory
func (m *Manager) ExtractArchive(ctx context.Context, archiveData []byte) (extractPath string, cleanup func(), err error) {
	// Create working directory
	workDir, cleanupDir, err := m.CreateWorkingDirectory()
	if err != nil {
		return "", nil, fmt.Errorf("failed to create working directory: %w", err)
	}
	
	// Create reader for gzipped data
	gzReader, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		cleanupDir()
		return "", nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()
	
	// Create tar reader
	tarReader := tar.NewReader(gzReader)
	
	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			cleanupDir()
			return "", nil, fmt.Errorf("failed to read tar header: %w", err)
		}
		
		// Validate and sanitize file path
		targetPath, err := m.sanitizePath(workDir, header.Name)
		if err != nil {
			cleanupDir()
			return "", nil, fmt.Errorf("invalid file path: %w", err)
		}
		
		switch header.Typeflag {
		case tar.TypeReg:
			// Regular file
			if err := m.extractFile(tarReader, targetPath, header.Mode); err != nil {
				cleanupDir()
				return "", nil, fmt.Errorf("failed to extract file %s: %w", header.Name, err)
			}
		case tar.TypeDir:
			// Directory
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				cleanupDir()
				return "", nil, fmt.Errorf("failed to create directory %s: %w", header.Name, err)
			}
		}
	}
	
	return workDir, cleanupDir, nil
}

// ExecuteCommand executes a command in the sandbox
func (m *Manager) ExecuteCommand(ctx context.Context, command string, args []string, workingDir string) (*ExecutionResult, error) {
	// Create command with context
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workingDir
	
	// Set up resource limits and security constraints
	if err := m.configureCommandSecurity(cmd); err != nil {
		return nil, fmt.Errorf("failed to configure command security: %w", err)
	}
	
	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	// Execute command
	err := cmd.Run()
	
	result := &ExecutionResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	
	// Get exit code
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to start command: %w", err)
		}
	} else {
		result.ExitCode = 0
	}
	
	return result, nil
}

// ValidateArchive validates an archive before extraction
func (m *Manager) ValidateArchive(archiveData []byte, allowedExtensions []string, maxSizeBytes int64) error {
	// Check archive size
	if int64(len(archiveData)) > maxSizeBytes {
		return fmt.Errorf("archive exceeds maximum size limit")
	}
	
	// Create reader for gzipped data
	gzReader, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return fmt.Errorf("failed to read archive: %w", err)
	}
	defer gzReader.Close()
	
	// Create tar reader
	tarReader := tar.NewReader(gzReader)
	
	// Validate each file in the archive
	totalSize := int64(0)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read archive: %w", err)
		}
		
		// Check file path for security issues
		if err := m.validateFilePath(header.Name); err != nil {
			return err
		}
		
		// Check file extension
		if header.Typeflag == tar.TypeReg {
			ext := filepath.Ext(header.Name)
			if !m.isAllowedExtension(ext, allowedExtensions) {
				return fmt.Errorf("file extension %s not allowed for file %s", ext, header.Name)
			}
		}
		
		// Track total extracted size
		totalSize += header.Size
		if totalSize > maxSizeBytes {
			return fmt.Errorf("archive exceeds maximum size after extraction")
		}
	}
	
	return nil
}

// CreateWorkingDirectory creates a temporary working directory
func (m *Manager) CreateWorkingDirectory() (workDir string, cleanup func(), err error) {
	workDir, err = os.MkdirTemp(m.workDir, "chttp-work-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create working directory: %w", err)
	}
	
	cleanup = func() {
		os.RemoveAll(workDir)
	}
	
	return workDir, cleanup, nil
}

// sanitizePath validates and sanitizes a file path to prevent directory traversal
func (m *Manager) sanitizePath(baseDir, filePath string) (string, error) {
	// Clean the file path
	cleaned := filepath.Clean(filePath)
	
	// Check for path traversal attempts
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path traversal detected in: %s", filePath)
	}
	
	// Check for absolute paths
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute path not allowed: %s", filePath)
	}
	
	// Join with base directory
	fullPath := filepath.Join(baseDir, cleaned)
	
	// Ensure the resulting path is still within the base directory
	if !strings.HasPrefix(fullPath, baseDir) {
		return "", fmt.Errorf("path outside working directory: %s", filePath)
	}
	
	return fullPath, nil
}

// extractFile extracts a single file from the tar archive
func (m *Manager) extractFile(reader io.Reader, targetPath string, mode int64) error {
	// Create directory if needed
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Create file
	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	
	// Copy data
	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("failed to write file data: %w", err)
	}
	
	return nil
}

// validateFilePath validates a file path for security
func (m *Manager) validateFilePath(filePath string) error {
	// Check for path traversal
	if strings.Contains(filePath, "..") {
		return fmt.Errorf("path traversal detected in: %s", filePath)
	}
	
	// Check for absolute paths
	if filepath.IsAbs(filePath) {
		return fmt.Errorf("absolute path not allowed: %s", filePath)
	}
	
	// Check for hidden files at root level (but allow in subdirectories)
	parts := strings.Split(filePath, "/")
	if len(parts) > 0 && strings.HasPrefix(parts[0], ".") {
		return fmt.Errorf("hidden files not allowed at root level: %s", filePath)
	}
	
	return nil
}

// isAllowedExtension checks if a file extension is in the allowed list
func (m *Manager) isAllowedExtension(ext string, allowed []string) bool {
	for _, allowedExt := range allowed {
		if ext == allowedExt {
			return true
		}
	}
	return false
}

// configureCommandSecurity configures security constraints for command execution
func (m *Manager) configureCommandSecurity(cmd *exec.Cmd) error {
	// Set environment variables for resource limits
	env := os.Environ()
	
	// Add memory limit if specified
	if m.maxMemory != "" {
		env = append(env, "CHTTP_MAX_MEMORY="+m.maxMemory)
	}
	
	// Add CPU limit if specified
	if m.maxCPU != "" {
		env = append(env, "CHTTP_MAX_CPU="+m.maxCPU)
	}
	
	cmd.Env = env
	
	// On Unix systems, we could set UID/GID here for user isolation
	// For now, we rely on the service being run as the appropriate user
	
	return nil
}

// parseMemoryLimit parses memory limit string to bytes
func parseMemoryLimit(limit string) (int64, error) {
	if limit == "" {
		return 0, nil
	}
	
	// Simple parser for formats like "512MB", "1GB"
	limit = strings.ToUpper(strings.TrimSpace(limit))
	
	multiplier := int64(1)
	if strings.HasSuffix(limit, "KB") {
		multiplier = 1024
		limit = strings.TrimSuffix(limit, "KB")
	} else if strings.HasSuffix(limit, "MB") {
		multiplier = 1024 * 1024
		limit = strings.TrimSuffix(limit, "MB")
	} else if strings.HasSuffix(limit, "GB") {
		multiplier = 1024 * 1024 * 1024
		limit = strings.TrimSuffix(limit, "GB")
	}
	
	value, err := strconv.ParseInt(limit, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory limit format: %s", limit)
	}
	
	return value * multiplier, nil
}
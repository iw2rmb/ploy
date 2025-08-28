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
	"time"
)

// ManagerConfig defines configuration for the sandbox manager
type ManagerConfig struct {
	WorkDir        string
	MaxMemory      string
	MaxCPUTime     string
	MaxProcesses   int
	CleanupTimeout string
}

// ResourceLimits contains parsed resource limit values
type ResourceLimits struct {
	MaxMemory      string
	MaxCPUTime     string
	MaxProcesses   int
	CleanupTimeout string
}

// ExecutionResult contains the result of a command execution
type ExecutionResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Manager handles secure execution environment management
type Manager struct {
	workDir        string
	maxMemory      string
	maxCPUTime     string
	maxProcesses   int
	cleanupTimeout string
	auditor        SecurityAuditor
}

// NewManager creates a new sandbox manager with the given configuration
func NewManager(config ManagerConfig) (*Manager, error) {
	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return &Manager{
		workDir:        config.WorkDir,
		maxMemory:      config.MaxMemory,
		maxCPUTime:     config.MaxCPUTime,
		maxProcesses:   config.MaxProcesses,
		cleanupTimeout: config.CleanupTimeout,
		auditor:        NewSecurityAuditor(1000), // Max 1000 log entries
	}, nil
}

// CreateWorkingDirectory creates a temporary working directory within the base work directory
func (m *Manager) CreateWorkingDirectory() (string, func(), error) {
	// Ensure base directory exists
	if err := os.MkdirAll(m.workDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	// Create temporary directory
	tempDir, err := os.MkdirTemp(m.workDir, "cllm-work-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create working directory: %w", err)
	}

	// Return cleanup function
	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup, nil
}

// ValidatePath validates that a file path is safe and within the allowed base directory
func (m *Manager) ValidatePath(baseDir, filePath string) (string, error) {
	if filePath == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Check for null bytes
	if strings.Contains(filePath, "\x00") {
		return "", fmt.Errorf("path contains null bytes")
	}

	// Clean the path
	cleanPath := filepath.Clean(filePath)

	// Check if path is absolute
	if filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("absolute paths not allowed")
	}

	// Check for path traversal
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("path traversal detected")
	}

	// Join with base directory
	fullPath := filepath.Join(baseDir, cleanPath)

	// Ensure the resulting path is still within base directory
	if !strings.HasPrefix(fullPath, baseDir+string(filepath.Separator)) && fullPath != baseDir {
		return "", fmt.Errorf("path outside base directory")
	}

	return fullPath, nil
}

// ParseMemoryLimit parses the memory limit string and returns bytes
func (m *Manager) ParseMemoryLimit() (int64, error) {
	return parseMemoryLimit(m.maxMemory)
}

// ParseCPUTimeLimit parses the CPU time limit string and returns duration
func (m *Manager) ParseCPUTimeLimit() (time.Duration, error) {
	if m.maxCPUTime == "" {
		return 0, nil
	}
	return time.ParseDuration(m.maxCPUTime)
}

// ParseCleanupTimeout parses the cleanup timeout string and returns duration
func (m *Manager) ParseCleanupTimeout() (time.Duration, error) {
	if m.cleanupTimeout == "" {
		return 30 * time.Second, nil // Default
	}
	return time.ParseDuration(m.cleanupTimeout)
}

// GetResourceLimits returns the current resource limits configuration
func (m *Manager) GetResourceLimits() ResourceLimits {
	return ResourceLimits{
		MaxMemory:      m.maxMemory,
		MaxCPUTime:     m.maxCPUTime,
		MaxProcesses:   m.maxProcesses,
		CleanupTimeout: m.cleanupTimeout,
	}
}

// Shutdown performs graceful shutdown of the manager
func (m *Manager) Shutdown(ctx context.Context) error {
	// In a real implementation, this would clean up any running processes
	// and temporary directories. For now, we just respect the context timeout.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Simulated cleanup time
		time.Sleep(10 * time.Millisecond)
		return nil
	}
}

// validateConfig validates the manager configuration
func validateConfig(config ManagerConfig) error {
	if config.WorkDir == "" {
		return fmt.Errorf("work directory cannot be empty")
	}

	// Validate memory format
	if config.MaxMemory != "" {
		if _, err := parseMemoryLimit(config.MaxMemory); err != nil {
			return fmt.Errorf("invalid memory format: %w", err)
		}
	}

	// Validate CPU time format
	if config.MaxCPUTime != "" {
		if _, err := time.ParseDuration(config.MaxCPUTime); err != nil {
			return fmt.Errorf("invalid CPU time format: %w", err)
		}
	}

	// Validate cleanup timeout format
	if config.CleanupTimeout != "" {
		if _, err := time.ParseDuration(config.CleanupTimeout); err != nil {
			return fmt.Errorf("invalid cleanup timeout format: %w", err)
		}
	}

	return nil
}

// parseMemoryLimit parses memory limit strings like "1GB", "512MB", etc.
func parseMemoryLimit(limit string) (int64, error) {
	if limit == "" {
		return 0, nil
	}

	limit = strings.ToUpper(strings.TrimSpace(limit))
	
	var multiplier int64 = 1
	var numStr string

	switch {
	case strings.HasSuffix(limit, "GB"):
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(limit, "GB")
	case strings.HasSuffix(limit, "MB"):
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(limit, "MB")
	case strings.HasSuffix(limit, "KB"):
		multiplier = 1024
		numStr = strings.TrimSuffix(limit, "KB")
	case strings.HasSuffix(limit, "B"):
		multiplier = 1
		numStr = strings.TrimSuffix(limit, "B")
	default:
		return 0, fmt.Errorf("invalid memory format: %s", limit)
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory format: %s", limit)
	}

	return num * multiplier, nil
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
		select {
		case <-ctx.Done():
			cleanupDir()
			return "", nil, fmt.Errorf("extraction cancelled: %w", ctx.Err())
		default:
			// Continue processing
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			cleanupDir()
			return "", nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		// Validate and sanitize file path
		targetPath, err := m.ValidatePath(workDir, header.Name)
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

// ExtractStreamingArchive extracts a gzipped tar archive from a reader stream
func (m *Manager) ExtractStreamingArchive(ctx context.Context, reader io.Reader) (extractPath string, cleanup func(), err error) {
	// Create working directory
	workDir, cleanupDir, err := m.CreateWorkingDirectory()
	if err != nil {
		return "", nil, fmt.Errorf("failed to create working directory: %w", err)
	}

	// Create gzip reader from stream
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		cleanupDir()
		return "", nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzReader)

	// Extract files streaming
	for {
		select {
		case <-ctx.Done():
			cleanupDir()
			return "", nil, fmt.Errorf("extraction cancelled: %w", ctx.Err())
		default:
			// Continue processing
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			cleanupDir()
			return "", nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		// Validate and sanitize file path
		targetPath, err := m.ValidatePath(workDir, header.Name)
		if err != nil {
			cleanupDir()
			return "", nil, fmt.Errorf("invalid file path: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeReg:
			// Regular file - stream directly to disk
			if err := m.extractFileStreaming(tarReader, targetPath, header.Mode); err != nil {
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

// ReadFileSecure reads a file from within the sandbox directory safely
func (m *Manager) ReadFileSecure(baseDir, relativePath string) ([]byte, error) {
	// Validate path
	fullPath, err := m.ValidatePath(baseDir, relativePath)
	if err != nil {
		return nil, err
	}

	// Read file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return content, nil
}

// WriteFileSecure writes a file within the sandbox directory safely
func (m *Manager) WriteFileSecure(baseDir, relativePath string, data []byte, mode os.FileMode) error {
	// Validate path
	fullPath, err := m.ValidatePath(baseDir, relativePath)
	if err != nil {
		m.auditor.LogOperation("file_write", fmt.Sprintf("path: %s, validation failed: %s", relativePath, err.Error()), "failed")
		return err
	}

	// Create directory if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		m.auditor.LogOperation("file_write", fmt.Sprintf("path: %s, directory creation failed", relativePath), "failed")
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, data, mode); err != nil {
		m.auditor.LogOperation("file_write", fmt.Sprintf("path: %s, write failed", relativePath), "failed")
		return fmt.Errorf("failed to write file: %w", err)
	}

	m.auditor.LogOperation("file_write", fmt.Sprintf("path: %s, size: %d bytes", relativePath, len(data)), "success")
	return nil
}

// ListDirectorySecure lists directory contents within the sandbox safely
func (m *Manager) ListDirectorySecure(baseDir, relativePath string) ([]os.DirEntry, error) {
	// Validate path
	fullPath, err := m.ValidatePath(baseDir, relativePath)
	if err != nil {
		return nil, err
	}

	// Read directory
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	return entries, nil
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

// extractFileStreaming extracts a file from a tar reader using streaming with a buffer
func (m *Manager) extractFileStreaming(reader io.Reader, targetPath string, mode int64) error {
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

	// Use a buffer for streaming copy
	buf := make([]byte, 32*1024) // 32KB buffer
	_, err = io.CopyBuffer(file, reader, buf)
	if err != nil {
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

// ExecuteCommand executes a command in the sandbox
func (m *Manager) ExecuteCommand(ctx context.Context, command string, args []string, workingDir string) (*ExecutionResult, error) {
	// Validate command arguments first
	if err := m.ValidateCommandArguments(command, args); err != nil {
		return nil, err
	}

	// Validate working directory
	if workingDir != "" {
		// For absolute paths, check if they're within the base sandbox directory
		if filepath.IsAbs(workingDir) {
			if !strings.HasPrefix(workingDir, m.workDir) {
				m.auditor.LogSecurityEvent("path_traversal_attempt", "high", "blocked", fmt.Sprintf("working directory outside sandbox: %s", workingDir), "command_execution")
				return nil, fmt.Errorf("working directory outside sandbox: %s", workingDir)
			}
		} else {
			// For relative paths, validate against path traversal
			_, err := m.ValidatePath(m.workDir, workingDir)
			if err != nil {
				m.auditor.LogSecurityEvent("path_traversal_attempt", "high", "blocked", fmt.Sprintf("invalid working directory: %s", err.Error()), "command_execution")
				return nil, fmt.Errorf("invalid working directory: %w", err)
			}
		}
	}

	// Create command with context
	cmd := exec.CommandContext(ctx, command, args...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

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
			m.auditor.LogOperation("command_execute", fmt.Sprintf("command: %s, args: %v, exit_code: %d", command, args, result.ExitCode), "completed")
		} else {
			m.auditor.LogOperation("command_execute", fmt.Sprintf("command: %s, args: %v", command, args), "failed_to_start")
			return nil, fmt.Errorf("failed to start command: %w", err)
		}
	} else {
		result.ExitCode = 0
		m.auditor.LogOperation("command_execute", fmt.Sprintf("command: %s, args: %v, exit_code: 0", command, args), "success")
	}

	return result, nil
}

// configureCommandSecurity configures security constraints for command execution
func (m *Manager) configureCommandSecurity(cmd *exec.Cmd) error {
	// Set environment variables for resource limits
	env := os.Environ()

	// Add memory limit if specified
	if m.maxMemory != "" {
		env = append(env, "CLLM_MAX_MEMORY="+m.maxMemory)
	}

	// Add CPU limit if specified
	if m.maxCPUTime != "" {
		env = append(env, "CLLM_MAX_CPU_TIME="+m.maxCPUTime)
	}

	// Add process limit if specified
	if m.maxProcesses > 0 {
		env = append(env, fmt.Sprintf("CLLM_MAX_PROCESSES=%d", m.maxProcesses))
	}

	cmd.Env = env

	// On Unix systems, we could set UID/GID here for user isolation
	// For now, we rely on the service being run as the appropriate user

	return nil
}

// GetSecurityAuditor returns the security auditor instance
func (m *Manager) GetSecurityAuditor() SecurityAuditor {
	return m.auditor
}

// ValidateCommandArguments validates command arguments for security issues
func (m *Manager) ValidateCommandArguments(command string, args []string) error {
	// Check for null bytes in command
	if strings.Contains(command, "\x00") {
		m.auditor.LogSecurityEvent("command_validation", "high", "blocked", "null byte detected in command", "command_validation")
		return fmt.Errorf("null byte detected in command")
	}
	
	for i, arg := range args {
		// Check for null bytes
		if strings.Contains(arg, "\x00") {
			m.auditor.LogSecurityEvent("argument_validation", "high", "blocked", fmt.Sprintf("null byte detected in argument %d", i), "command_validation")
			return fmt.Errorf("null byte detected in argument %d", i)
		}
		
		// Check for suspicious command injection patterns
		suspiciousPatterns := []string{
			"$(", "`", ";", "&&", "||", "|", ">", "<", "&",
		}
		
		for _, pattern := range suspiciousPatterns {
			if strings.Contains(arg, pattern) {
				m.auditor.LogSecurityEvent("command_injection", "high", "blocked", fmt.Sprintf("suspicious pattern '%s' in argument %d: %s", pattern, i, arg), "command_validation")
				return fmt.Errorf("suspicious command injection pattern detected in argument %d", i)
			}
		}
		
		// Check for path traversal in arguments
		if strings.Contains(arg, "..") {
			m.auditor.LogSecurityEvent("path_traversal_arg", "medium", "blocked", fmt.Sprintf("path traversal detected in argument %d: %s", i, arg), "command_validation")
			return fmt.Errorf("path traversal detected in argument %d", i)
		}
		
		// Check argument length
		if len(arg) > 4096 {
			m.auditor.LogSecurityEvent("argument_length", "medium", "blocked", fmt.Sprintf("argument %d exceeds length limit", i), "command_validation")
			return fmt.Errorf("argument too long: argument %d exceeds 4096 characters", i)
		}
	}
	
	return nil
}

// DetectSymlink detects if a path is a symlink and returns its target
func (m *Manager) DetectSymlink(path string) (bool, string, error) {
	fileInfo, err := os.Lstat(path)
	if err != nil {
		return false, "", err
	}
	
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return true, "", err
		}
		return true, target, nil
	}
	
	return false, "", nil
}

// ValidateSymlinkTarget validates that a symlink target is within sandbox boundaries
func (m *Manager) ValidateSymlinkTarget(baseDir, target string) error {
	// Resolve absolute path
	var absTarget string
	if filepath.IsAbs(target) {
		absTarget = target
	} else {
		absTarget = filepath.Join(baseDir, target)
	}
	
	// Clean the path
	absTarget = filepath.Clean(absTarget)
	
	// Check if target is within sandbox
	if !strings.HasPrefix(absTarget, baseDir) {
		m.auditor.LogSecurityEvent("symlink_escape", "high", "blocked", fmt.Sprintf("symlink target outside sandbox: %s", target), "symlink_validation")
		return fmt.Errorf("symlink target outside sandbox: %s", target)
	}
	
	return nil
}

// ValidatePathEnhanced provides enhanced path validation with additional security checks
func (m *Manager) ValidatePathEnhanced(baseDir, filePath string) (string, error) {
	// First run standard validation
	fullPath, err := m.ValidatePath(baseDir, filePath)
	if err != nil {
		return "", err
	}
	
	// Additional enhanced checks
	
	// Check path length
	if len(filePath) > 255 {
		m.auditor.LogSecurityEvent("path_length", "medium", "blocked", fmt.Sprintf("path length exceeds limit: %d chars", len(filePath)), "path_validation")
		return "", fmt.Errorf("path length exceeds limit: %d characters", len(filePath))
	}
	
	// Check path depth
	parts := strings.Split(strings.Trim(filePath, "/"), "/")
	if len(parts) > 20 {
		m.auditor.LogSecurityEvent("path_depth", "medium", "blocked", fmt.Sprintf("path depth exceeds limit: %d levels", len(parts)), "path_validation")
		return "", fmt.Errorf("path depth exceeds limit: %d levels", len(parts))
	}
	
	// Check for control characters
	for _, char := range filePath {
		if char < 32 || char == 127 {
			m.auditor.LogSecurityEvent("control_characters", "medium", "blocked", "path contains control characters", "path_validation")
			return "", fmt.Errorf("path contains control characters")
		}
	}
	
	// Check for invalid path separators (Windows-style backslashes)
	if strings.Contains(filePath, "\\") {
		m.auditor.LogSecurityEvent("invalid_separators", "low", "blocked", "path contains backslashes", "path_validation")
		return "", fmt.Errorf("path contains invalid path separators")
	}
	
	return fullPath, nil
}

// StartResourceMonitoring starts monitoring resource usage
func (m *Manager) StartResourceMonitoring() (ResourceMonitor, error) {
	monitor := NewResourceMonitor(m)
	m.auditor.LogOperation("resource_monitoring", "started resource monitoring", "success")
	return monitor, nil
}
package security

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

// ResourceLimits defines resource constraints for processes
type ResourceLimits struct {
	MaxCPU        float64       // CPU cores (e.g., 0.5 = 50% of one core)
	MaxMemory     int64         // Memory in bytes
	MaxFiles      int           // Maximum open files
	MaxDuration   time.Duration // Maximum execution time
	EnableCgroups bool          // Use cgroups on Linux
	CgroupName    string        // Name of cgroup to create
}

// ResourceLimiter applies resource limits to processes
type ResourceLimiter struct {
	limits ResourceLimits
	mu     sync.Mutex
}

// NewResourceLimiter creates a new resource limiter
func NewResourceLimiter(limits ResourceLimits) *ResourceLimiter {
	return &ResourceLimiter{
		limits: limits,
	}
}

// WrapCommand wraps a command with resource limits
func (r *ResourceLimiter) WrapCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	// Create command with timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, r.limits.MaxDuration)
	cmd := exec.CommandContext(timeoutCtx, name, args...)
	
	// Apply resource limits based on OS
	if runtime.GOOS == "linux" {
		r.applyLinuxLimits(cmd)
	} else {
		r.applyUnixLimits(cmd)
	}
	
	// Ensure cleanup happens
	go func() {
		<-timeoutCtx.Done()
		cancel()
	}()
	
	return cmd
}

// applyLinuxLimits applies limits on Linux using cgroups and ulimit
func (r *ResourceLimiter) applyLinuxLimits(cmd *exec.Cmd) {
	if r.limits.EnableCgroups {
		r.setupCgroup(cmd)
	}
	
	// Set ulimits via SysProcAttr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	
	// Apply memory and file limits
	r.applyUlimits(cmd)
}

// applyUnixLimits applies limits on Unix-like systems (macOS, BSD)
func (r *ResourceLimiter) applyUnixLimits(cmd *exec.Cmd) {
	// Use ulimit for resource limits
	r.applyUlimits(cmd)
}

// applyUlimits applies ulimit-based resource limits
func (r *ResourceLimiter) applyUlimits(cmd *exec.Cmd) {
	// Wrap command with ulimit
	ulimitArgs := []string{}
	
	// Memory limit (in KB)
	if r.limits.MaxMemory > 0 {
		memKB := r.limits.MaxMemory / 1024
		ulimitArgs = append(ulimitArgs, fmt.Sprintf("ulimit -v %d;", memKB))
	}
	
	// File limit
	if r.limits.MaxFiles > 0 {
		ulimitArgs = append(ulimitArgs, fmt.Sprintf("ulimit -n %d;", r.limits.MaxFiles))
	}
	
	// CPU time limit (approximate)
	if r.limits.MaxCPU > 0 && r.limits.MaxDuration > 0 {
		cpuSeconds := int(float64(r.limits.MaxDuration.Seconds()) * r.limits.MaxCPU)
		ulimitArgs = append(ulimitArgs, fmt.Sprintf("ulimit -t %d;", cpuSeconds))
	}
	
	if len(ulimitArgs) > 0 {
		// Wrap original command with ulimit
		shellCmd := strings.Join(ulimitArgs, " ") + " " + shellQuoteCommand(cmd.Path, cmd.Args[1:]...)
		cmd.Path = "/bin/sh"
		cmd.Args = []string{"sh", "-c", shellCmd}
	}
}

// setupCgroup creates and configures a cgroup for the process
func (r *ResourceLimiter) setupCgroup(cmd *exec.Cmd) {
	if r.limits.CgroupName == "" {
		r.limits.CgroupName = fmt.Sprintf("chttp_%d", os.Getpid())
	}
	
	cgroupPath := fmt.Sprintf("/sys/fs/cgroup/cpu/%s", r.limits.CgroupName)
	
	// Create cgroup
	os.MkdirAll(cgroupPath, 0755)
	
	// Set CPU limit
	if r.limits.MaxCPU > 0 {
		quota := int64(r.limits.MaxCPU * 100000) // Convert to microseconds
		os.WriteFile(filepath.Join(cgroupPath, "cpu.cfs_quota_us"), []byte(fmt.Sprintf("%d", quota)), 0644)
		os.WriteFile(filepath.Join(cgroupPath, "cpu.cfs_period_us"), []byte("100000"), 0644)
	}
	
	// Store cgroup path for cleanup
	cmd.Env = append(cmd.Env, fmt.Sprintf("CHTTP_CGROUP_PATH=%s", cgroupPath))
}

// shellQuoteCommand quotes a command for shell execution
func shellQuoteCommand(name string, args ...string) string {
	quoted := []string{shellQuote(name)}
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

// shellQuote quotes a single argument for shell
func shellQuote(s string) string {
	if strings.ContainsAny(s, " \t\n'\"\\$") {
		return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
	}
	return s
}

// RateLimitConfig configures rate limiting
type RateLimitConfig struct {
	RequestsPerSecond float64
	BurstSize         int
	PerClient         bool // If true, limits apply per client
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	config    RateLimitConfig
	global    *rate.Limiter
	clients   map[string]*rate.Limiter
	clientsMu sync.RWMutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config:  config,
		clients: make(map[string]*rate.Limiter),
	}
	
	if !config.PerClient {
		rl.global = rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.BurstSize)
	}
	
	return rl
}

// Allow checks if a request from the client is allowed
func (r *RateLimiter) Allow(clientID string) bool {
	if !r.config.PerClient {
		if r.global == nil {
			return true
		}
		return r.global.Allow()
	}
	
	// Per-client limiting
	r.clientsMu.RLock()
	limiter, exists := r.clients[clientID]
	r.clientsMu.RUnlock()
	
	if !exists {
		r.clientsMu.Lock()
		limiter = rate.NewLimiter(rate.Limit(r.config.RequestsPerSecond), r.config.BurstSize)
		r.clients[clientID] = limiter
		r.clientsMu.Unlock()
	}
	
	return limiter.Allow()
}

// PathSanitizer validates and sanitizes file paths
type PathSanitizer struct {
	baseDir string
}

// NewPathSanitizer creates a new path sanitizer
func NewPathSanitizer(baseDir string) *PathSanitizer {
	absBase, _ := filepath.Abs(baseDir)
	return &PathSanitizer{
		baseDir: absBase,
	}
}

// Sanitize validates and sanitizes a path
func (s *PathSanitizer) Sanitize(path string) (string, error) {
	// Reject absolute paths
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path traversal detected: absolute path not allowed")
	}
	
	// Reject paths with ..
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("path traversal detected: '..' not allowed")
	}
	
	// Clean the path
	cleanPath := filepath.Clean(path)
	
	// Join with base directory
	fullPath := filepath.Join(s.baseDir, cleanPath)
	
	// Resolve any symlinks
	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("path traversal detected: symlink resolution failed")
	}
	
	// If path doesn't exist yet, use the cleaned version
	if os.IsNotExist(err) {
		resolvedPath = fullPath
	}
	
	// Ensure resolved path is within base directory
	if !strings.HasPrefix(resolvedPath, s.baseDir) {
		return "", fmt.Errorf("path traversal detected: resolved path outside base directory")
	}
	
	return resolvedPath, nil
}

// ValidateDirectory validates all files in a directory
func (s *PathSanitizer) ValidateDirectory(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Check for symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			
			// Resolve symlink target
			absTarget := target
			if !filepath.IsAbs(target) {
				absTarget = filepath.Join(filepath.Dir(path), target)
			}
			
			// Check if symlink escapes directory
			resolved, _ := filepath.Abs(absTarget)
			if !strings.HasPrefix(resolved, dir) {
				return fmt.Errorf("dangerous symlink detected: %s -> %s", path, target)
			}
		}
		
		return nil
	})
}

// SecurityConfig defines security validation rules
type SecurityConfig struct {
	MaxArchiveSize    int64
	MaxExtractedSize  int64
	MaxFiles          int
	AllowedExtensions []string
	BlockedPaths      []string
}

// ArchiveMetadata contains archive information
type ArchiveMetadata struct {
	Size          int64
	ExtractedSize int64
	FileCount     int
	Files         []string
}

// SecurityValidator validates archives against security rules
type SecurityValidator struct {
	config SecurityConfig
}

// NewSecurityValidator creates a new security validator
func NewSecurityValidator(config SecurityConfig) *SecurityValidator {
	return &SecurityValidator{
		config: config,
	}
}

// ValidateArchive validates an archive against security rules
func (v *SecurityValidator) ValidateArchive(metadata ArchiveMetadata) error {
	// Check archive size
	if metadata.Size > v.config.MaxArchiveSize {
		return fmt.Errorf("archive size exceeds limit: %d > %d", metadata.Size, v.config.MaxArchiveSize)
	}
	
	// Check extracted size
	if metadata.ExtractedSize > v.config.MaxExtractedSize {
		return fmt.Errorf("extracted size exceeds limit: %d > %d", metadata.ExtractedSize, v.config.MaxExtractedSize)
	}
	
	// Check file count
	if metadata.FileCount > v.config.MaxFiles {
		return fmt.Errorf("too many files: %d > %d", metadata.FileCount, v.config.MaxFiles)
	}
	
	// Check individual files
	for _, file := range metadata.Files {
		// Check extension
		if !v.isExtensionAllowed(file) {
			return fmt.Errorf("blocked file extension: %s", file)
		}
		
		// Check path
		if v.isPathBlocked(file) {
			return fmt.Errorf("blocked path: %s", file)
		}
	}
	
	return nil
}

// isExtensionAllowed checks if file extension is allowed
func (v *SecurityValidator) isExtensionAllowed(filename string) bool {
	if len(v.config.AllowedExtensions) == 0 {
		return true // No restrictions
	}
	
	ext := filepath.Ext(filename)
	for _, allowed := range v.config.AllowedExtensions {
		if ext == allowed {
			return true
		}
	}
	
	return false
}

// isPathBlocked checks if path contains blocked components
func (v *SecurityValidator) isPathBlocked(path string) bool {
	for _, blocked := range v.config.BlockedPaths {
		if strings.Contains(path, blocked) {
			return true
		}
	}
	return false
}
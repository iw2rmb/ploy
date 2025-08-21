package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// toJSON converts an object to JSON bytes
func toJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// parseJSON parses JSON bytes into an object
func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Copy permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to get source file info: %w", err)
	}

	err = os.Chmod(dst, sourceInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	return nil
}

// getDiskSpace returns available disk space in bytes for the given path
func getDiskSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, fmt.Errorf("failed to get disk space for %s: %w", path, err)
	}

	// Available space = available blocks * block size
	available := int64(stat.Bavail) * int64(stat.Bsize)
	return available, nil
}

// isNewerVersion compares two version strings
// Returns true if version1 is newer than version2
func isNewerVersion(version1, version2 string) bool {
	// Handle special cases
	if version1 == version2 {
		return false
	}

	// Simple semantic version comparison
	v1Parts := parseVersion(version1)
	v2Parts := parseVersion(version2)

	// Compare each part
	for i := 0; i < len(v1Parts) && i < len(v2Parts); i++ {
		if v1Parts[i] > v2Parts[i] {
			return true
		} else if v1Parts[i] < v2Parts[i] {
			return false
		}
	}

	// If all compared parts are equal, the longer version is newer
	return len(v1Parts) > len(v2Parts)
}

// parseVersion parses a version string into comparable parts
func parseVersion(version string) []int {
	// Remove common prefixes
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "version")

	// Handle special test versions
	if strings.Contains(version, "test-") {
		// For test versions, use timestamp comparison
		parts := strings.Split(version, "-")
		if len(parts) >= 2 {
			// Extract timestamp part and convert to comparable format
			timestamp := parts[len(parts)-1]
			if len(timestamp) >= 8 { // YYYYMMDD format or longer
				if val, err := strconv.Atoi(timestamp[:8]); err == nil {
					return []int{0, 0, val} // Treat as 0.0.YYYYMMDD
				}
			}
		}
		return []int{0, 0, 1} // Default for test versions
	}

	// Regular version parsing (semantic versioning)
	versionRegex := regexp.MustCompile(`(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-(.+))?`)
	matches := versionRegex.FindStringSubmatch(version)

	if matches == nil {
		// Fallback: lexicographic comparison as numbers
		if val, err := strconv.Atoi(version); err == nil {
			return []int{val}
		}
		return []int{0}
	}

	parts := make([]int, 0, 3)

	// Major version
	if matches[1] != "" {
		if major, err := strconv.Atoi(matches[1]); err == nil {
			parts = append(parts, major)
		}
	}

	// Minor version
	if matches[2] != "" {
		if minor, err := strconv.Atoi(matches[2]); err == nil {
			parts = append(parts, minor)
		}
	} else {
		parts = append(parts, 0)
	}

	// Patch version
	if matches[3] != "" {
		if patch, err := strconv.Atoi(matches[3]); err == nil {
			parts = append(parts, patch)
		}
	} else {
		parts = append(parts, 0)
	}

	// Pre-release versions are considered older
	if matches[4] != "" {
		// Subtract 1 from patch version for pre-release
		if len(parts) > 2 && parts[2] > 0 {
			parts[2]--
		}
	}

	return parts
}

// GetCurrentVersion attempts to determine the current controller version
func GetCurrentVersion() string {
	// Try to get version from environment variable first
	if version := os.Getenv("PLOY_CONTROLLER_VERSION"); version != "" {
		return version
	}

	// Try to get from build-time variable (would need to be set during build)
	// This would require build-time injection via ldflags
	// For now, return a default
	return "unknown"
}

// ValidateVersionFormat validates version string format
func ValidateVersionFormat(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}

	if len(version) > 50 {
		return fmt.Errorf("version string too long (max 50 characters)")
	}

	// Allow alphanumeric, dots, hyphens, and underscores
	validChars := regexp.MustCompile(`^[a-zA-Z0-9.-_]+$`)
	if !validChars.MatchString(version) {
		return fmt.Errorf("version contains invalid characters")
	}

	return nil
}

// GetExecutablePath returns the path to the current executable
func GetExecutablePath() (string, error) {
	executable := os.Args[0]
	if executable == "" {
		return "", fmt.Errorf("unable to determine executable path")
	}
	return executable, nil
}

// GetControllerInfo returns information about the running controller
func GetControllerInfo() map[string]interface{} {
	executable, _ := GetExecutablePath()
	
	info := map[string]interface{}{
		"version":      GetCurrentVersion(),
		"executable":   executable,
		"pid":          os.Getpid(),
		"platform":     "linux",  // TODO: detect runtime platform
		"architecture": "amd64",  // TODO: detect runtime architecture
	}

	// Add file info if possible
	if stat, err := os.Stat(executable); err == nil {
		info["size"] = stat.Size()
		info["mode"] = stat.Mode().String()
		info["modified"] = stat.ModTime()
	}

	return info
}
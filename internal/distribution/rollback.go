package distribution

import (
	"fmt"
	"sort"
	"time"
)

// RollbackManager handles controller binary rollbacks
type RollbackManager struct {
	distributor *BinaryDistributor
	platform    string
	architecture string
}

// NewRollbackManager creates a new rollback manager
func NewRollbackManager(distributor *BinaryDistributor, platform, architecture string) *RollbackManager {
	return &RollbackManager{
		distributor:  distributor,
		platform:    platform,
		architecture: architecture,
	}
}

// RollbackInfo contains information about a rollback operation
type RollbackInfo struct {
	FromVersion string            `json:"from_version"`
	ToVersion   string            `json:"to_version"`
	Timestamp   time.Time         `json:"timestamp"`
	Reason      string            `json:"reason"`
	Success     bool              `json:"success"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// GetRollbackTargets returns available versions to rollback to
func (rm *RollbackManager) GetRollbackTargets(currentVersion string) ([]string, error) {
	versions, err := rm.distributor.ListVersions()
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}
	
	// Filter out current version and sort
	var targets []string
	for _, version := range versions {
		if version != currentVersion {
			targets = append(targets, version)
		}
	}
	
	// Sort versions (most recent first)
	sort.Slice(targets, func(i, j int) bool {
		return compareVersions(targets[i], targets[j]) > 0
	})
	
	return targets, nil
}

// RollbackTo performs a rollback to a specific version
func (rm *RollbackManager) RollbackTo(currentVersion, targetVersion, reason string) (*RollbackInfo, error) {
	rollback := &RollbackInfo{
		FromVersion: currentVersion,
		ToVersion:   targetVersion,
		Timestamp:   time.Now(),
		Reason:      reason,
		Metadata:    make(map[string]string),
	}
	
	// Download target version
	binaryPath, info, err := rm.distributor.DownloadBinary(targetVersion, rm.platform, rm.architecture)
	if err != nil {
		rollback.Success = false
		rollback.Error = fmt.Sprintf("failed to download target version: %v", err)
		return rollback, fmt.Errorf("rollback failed: %w", err)
	}
	
	// Validate target version
	if err := rm.validateRollbackTarget(info); err != nil {
		rollback.Success = false
		rollback.Error = fmt.Sprintf("validation failed: %v", err)
		return rollback, fmt.Errorf("rollback validation failed: %w", err)
	}
	
	// Add metadata
	rollback.Metadata["target_binary_path"] = binaryPath
	rollback.Metadata["target_binary_hash"] = info.SHA256Hash
	rollback.Metadata["target_build_time"] = info.BuildTime.Format(time.RFC3339)
	rollback.Metadata["target_git_commit"] = info.GitCommit
	
	rollback.Success = true
	return rollback, nil
}

// GetLastKnownGood returns the last known good version before current
func (rm *RollbackManager) GetLastKnownGood(currentVersion string) (string, error) {
	targets, err := rm.GetRollbackTargets(currentVersion)
	if err != nil {
		return "", err
	}
	
	if len(targets) == 0 {
		return "", fmt.Errorf("no rollback targets available")
	}
	
	// Return most recent version (first in sorted list)
	return targets[0], nil
}

// ValidateRollback checks if rollback is safe
func (rm *RollbackManager) ValidateRollback(fromVersion, toVersion string) error {
	// Check if target version exists
	_, _, err := rm.distributor.DownloadBinary(toVersion, rm.platform, rm.architecture)
	if err != nil {
		return fmt.Errorf("target version %s not available: %w", toVersion, err)
	}
	
	// Check version compatibility (basic check)
	if compareVersions(toVersion, fromVersion) > 0 {
		return fmt.Errorf("cannot rollback to newer version %s from %s", toVersion, fromVersion)
	}
	
	return nil
}

// validateRollbackTarget validates a binary before rollback
func (rm *RollbackManager) validateRollbackTarget(info *BinaryInfo) error {
	if err := info.Validate(); err != nil {
		return fmt.Errorf("invalid binary info: %w", err)
	}
	
	if !info.IsCompatibleWith(rm.platform, rm.architecture) {
		return fmt.Errorf("binary not compatible with %s/%s", rm.platform, rm.architecture)
	}
	
	// Check if binary is too old (more than 1 year)
	if time.Since(info.BuildTime) > 365*24*time.Hour {
		return fmt.Errorf("binary is too old (built %s)", info.BuildTime.Format("2006-01-02"))
	}
	
	return nil
}

// compareVersions compares two version strings
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	// Simple lexicographic comparison for now
	// In production, use proper semantic versioning
	if v1 > v2 {
		return 1
	} else if v1 < v2 {
		return -1
	}
	return 0
}

// CreateEmergencyRollback creates an emergency rollback plan
func (rm *RollbackManager) CreateEmergencyRollback(currentVersion string) (*RollbackInfo, error) {
	lastKnownGood, err := rm.GetLastKnownGood(currentVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to find last known good version: %w", err)
	}
	
	return rm.RollbackTo(currentVersion, lastKnownGood, "emergency rollback")
}
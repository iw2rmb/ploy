package arf

import (
	"fmt"
	"strconv"
	"strings"
)

// GenerateAttemptPath generates the next attempt path based on parent path and existing attempts
// For root attempts (parentPath=""), it generates "1", "2", "3", etc.
// For child attempts, it generates "1.1", "1.2", "2.1", etc.
func GenerateAttemptPath(rootID string, parentPath string, existing []HealingAttempt) string {
	nextNum := GetNextSiblingNumber(existing, parentPath)

	if parentPath == "" {
		// Root level attempt
		return strconv.Itoa(nextNum)
	}

	// Child attempt
	return fmt.Sprintf("%s.%d", parentPath, nextNum)
}

// GetNextSiblingNumber determines the next sibling number for a given parent path
func GetNextSiblingNumber(children []HealingAttempt, parentPath string) int {
	if parentPath == "" {
		// Looking for root level siblings
		maxNum := 0
		for _, child := range children {
			// Only consider direct root children (no dots in path)
			if !strings.Contains(child.AttemptPath, ".") {
				if num, err := strconv.Atoi(child.AttemptPath); err == nil && num > maxNum {
					maxNum = num
				}
			}
		}
		return maxNum + 1
	}

	// Find the parent attempt and count its children
	parent := FindAttemptByPath(children, parentPath)
	if parent == nil {
		return 1 // First child of this parent
	}

	maxNum := 0
	for _, child := range parent.Children {
		// Extract the last number from the child's path
		parts := strings.Split(child.AttemptPath, ".")
		if len(parts) > 0 {
			if num, err := strconv.Atoi(parts[len(parts)-1]); err == nil && num > maxNum {
				maxNum = num
			}
		}
	}

	return maxNum + 1
}

// ValidateAttemptPath validates that an attempt path is properly formatted
// Valid paths: "1", "2", "1.1", "1.2.3", etc.
// Invalid paths: "", ".1", "1.", "1..2", "1.0", "1.a", etc.
func ValidateAttemptPath(path string) error {
	if path == "" {
		return fmt.Errorf("attempt path cannot be empty")
	}

	if strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") {
		return fmt.Errorf("attempt path cannot start or end with a dot: %s", path)
	}

	if strings.Contains(path, "..") {
		return fmt.Errorf("attempt path cannot contain consecutive dots: %s", path)
	}

	parts := strings.Split(path, ".")
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("attempt path has empty segment at position %d: %s", i, path)
		}

		num, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("attempt path segment '%s' is not a valid number: %s", part, path)
		}

		if num <= 0 {
			return fmt.Errorf("attempt path numbers must be positive (got %d): %s", num, path)
		}
	}

	return nil
}

// GetPathDepth returns the depth of an attempt path
// "1" returns 1, "1.2" returns 2, "1.2.3" returns 3, etc.
func GetPathDepth(path string) int {
	if path == "" {
		return 0
	}

	return strings.Count(path, ".") + 1
}

// GetParentPath extracts the parent path from a child path
// "1" returns "", "1.2" returns "1", "1.2.3" returns "1.2", etc.
func GetParentPath(path string) string {
	if path == "" {
		return ""
	}

	lastDot := strings.LastIndex(path, ".")
	if lastDot == -1 {
		// Root level path, no parent
		return ""
	}

	return path[:lastDot]
}

// FindAttemptByPath recursively searches for an attempt with the given path
func FindAttemptByPath(attempts []HealingAttempt, path string) *HealingAttempt {
	for i := range attempts {
		if attempts[i].AttemptPath == path {
			return &attempts[i]
		}

		// Recursively search in children
		if found := FindAttemptByPath(attempts[i].Children, path); found != nil {
			return found
		}
	}

	return nil
}

// GenerateNextPath is a convenience function that generates the next attempt path
// for a transformation by fetching the current status and calculating the next path
func GenerateNextPath(store ConsulStoreInterface, rootID string, parentPath string) (string, error) {
	status, err := store.GetTransformationStatus(nil, rootID)
	if err != nil {
		return "", fmt.Errorf("failed to get transformation status: %w", err)
	}

	if status == nil {
		// No existing transformation, this would be the first attempt
		if parentPath != "" {
			return "", fmt.Errorf("cannot create child attempt for non-existent transformation")
		}
		return "1", nil
	}

	return GenerateAttemptPath(rootID, parentPath, status.Children), nil
}

// IsValidParent checks if a given path exists and can have children
func IsValidParent(attempts []HealingAttempt, parentPath string) bool {
	if parentPath == "" {
		// Empty parent is always valid (root level)
		return true
	}

	parent := FindAttemptByPath(attempts, parentPath)
	return parent != nil
}

// GetAllAttemptPaths returns all attempt paths in the tree (for validation/debugging)
func GetAllAttemptPaths(attempts []HealingAttempt) []string {
	var paths []string

	var collect func([]HealingAttempt)
	collect = func(attempts []HealingAttempt) {
		for _, attempt := range attempts {
			paths = append(paths, attempt.AttemptPath)
			if len(attempt.Children) > 0 {
				collect(attempt.Children)
			}
		}
	}

	collect(attempts)
	return paths
}

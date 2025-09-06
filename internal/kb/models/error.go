// Package models provides core data structures for the Knowledge Base system.
//
// This package defines the foundational types used throughout the KB system:
// Error patterns with canonical signatures, learning Cases that link errors
// to patch outcomes, and Summary statistics for aggregated learning data.
//
// Key types:
//   - Error: Canonical error pattern with signature generation
//   - Case: Individual learning instance with patch and success data
//   - Summary: Aggregated statistics and top-performing patches
//
// Integration points:
//   - Used by storage layer for persistence operations
//   - Used by learning pipeline for case management
//   - Used by fingerprint module for patch analysis
//
// See Also:
//   - internal/kb/storage: For persistence operations
//   - internal/kb/learning: For learning pipeline integration
package models

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Error represents a canonical error pattern in the KB
type Error struct {
	ID        string    `json:"id"`         // Generated from signature
	Signature string    `json:"signature"`  // Canonical error pattern
	Message   string    `json:"message"`    // Original error message
	Location  string    `json:"location"`   // File:line where error occurred
	BuildLogs []string  `json:"build_logs"` // Relevant log excerpts
	Created   time.Time `json:"created"`
}

// NewError creates a new Error with generated signature
func NewError(message, location string, buildLogs []string) *Error {
	e := &Error{
		Message:   message,
		Location:  location,
		BuildLogs: buildLogs,
		Created:   time.Now(),
	}

	e.Signature = e.GenerateSignature()
	e.ID = e.Signature

	return e
}

// GenerateSignature creates a canonical signature for this error
func (e *Error) GenerateSignature() string {
	normalized := e.NormalizeMessage()

	// Extract error patterns
	if strings.Contains(normalized, "cannot find symbol") {
		return "java-compilation-missing-symbol"
	}
	if strings.Contains(normalized, "expected") {
		return "java-syntax-semicolon"
	}

	// Fallback: hash the normalized message
	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("error-%x", hash[:8])
}

// NormalizeMessage normalizes the error message for pattern matching
func (e *Error) NormalizeMessage() string {
	msg := e.Message

	// Remove type parameters like <String>
	re := regexp.MustCompile(`<[^>]+>`)
	msg = re.ReplaceAllString(msg, "")

	// Normalize whitespace
	re = regexp.MustCompile(`\s+`)
	msg = re.ReplaceAllString(msg, " ")

	return strings.TrimSpace(msg)
}

// NormalizeBuildLogs filters and normalizes build logs
func (e *Error) NormalizeBuildLogs() []string {
	var normalized []string

	for _, log := range e.BuildLogs {
		// Keep only error logs
		if !strings.Contains(log, "[ERROR]") {
			continue
		}

		// Normalize file paths - remove directory prefixes
		re := regexp.MustCompile(`/[^/]*?([^/]+\.java)`)
		log = re.ReplaceAllString(log, "$1")

		normalized = append(normalized, log)
	}

	return normalized
}

// Validate checks if the error is valid
func (e *Error) Validate() error {
	if e.Signature == "" {
		return fmt.Errorf("signature cannot be empty")
	}
	if e.Message == "" {
		return fmt.Errorf("message cannot be empty")
	}
	return nil
}

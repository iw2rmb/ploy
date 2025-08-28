package diff

import (
	"fmt"
	"time"
)

// DiffRequest represents a request to generate a diff
type DiffRequest struct {
	// Original and modified content
	Original string `json:"original"`
	Modified string `json:"modified"`
	
	// File metadata
	OriginalPath string    `json:"original_path,omitempty"`
	ModifiedPath string    `json:"modified_path,omitempty"`
	OriginalTime time.Time `json:"original_time,omitempty"`
	ModifiedTime time.Time `json:"modified_time,omitempty"`
	
	// Options
	Options DiffOptions `json:"options,omitempty"`
}

// DiffOptions configures diff generation
type DiffOptions struct {
	// Context lines around changes
	ContextLines int `json:"context_lines,omitempty"`
	
	// Output format
	Format DiffFormat `json:"format,omitempty"`
	
	// Include statistics
	IncludeStats bool `json:"include_stats,omitempty"`
	
	// Ignore whitespace changes
	IgnoreWhitespace bool `json:"ignore_whitespace,omitempty"`
	
	// Binary file handling
	TreatAsBinary bool `json:"treat_as_binary,omitempty"`
}

// DiffFormat represents the output format for diffs
type DiffFormat string

const (
	// FormatUnified is the standard unified diff format
	FormatUnified DiffFormat = "unified"
	
	// FormatJSON is a structured JSON representation
	FormatJSON DiffFormat = "json"
	
	// FormatSummary is a human-readable summary
	FormatSummary DiffFormat = "summary"
)

// DiffResponse represents a generated diff
type DiffResponse struct {
	// The generated diff content
	Content string `json:"content"`
	
	// Format of the diff
	Format DiffFormat `json:"format"`
	
	// Statistics about the diff
	Stats *DiffStats `json:"stats,omitempty"`
	
	// Individual file changes
	Changes []FileChange `json:"changes,omitempty"`
	
	// Metadata
	Metadata DiffMetadata `json:"metadata"`
}

// DiffStats contains statistics about a diff
type DiffStats struct {
	// File statistics
	FilesChanged int `json:"files_changed"`
	FilesAdded   int `json:"files_added"`
	FilesDeleted int `json:"files_deleted"`
	
	// Line statistics
	LinesAdded   int `json:"lines_added"`
	LinesDeleted int `json:"lines_deleted"`
	LinesChanged int `json:"lines_changed"`
	
	// Binary files
	BinaryFiles int `json:"binary_files,omitempty"`
}

// DiffMetadata contains metadata about diff generation
type DiffMetadata struct {
	// Generation timestamp
	GeneratedAt time.Time `json:"generated_at"`
	
	// Generator version
	GeneratorVersion string `json:"generator_version"`
	
	// Processing time
	ProcessingTime time.Duration `json:"processing_time"`
	
	// Any warnings during generation
	Warnings []string `json:"warnings,omitempty"`
}

// FileChange represents changes to a single file
type FileChange struct {
	// File paths
	OriginalPath string `json:"original_path"`
	ModifiedPath string `json:"modified_path"`
	
	// Change type
	ChangeType ChangeType `json:"change_type"`
	
	// File mode changes
	OldMode string `json:"old_mode,omitempty"`
	NewMode string `json:"new_mode,omitempty"`
	
	// Binary file indicator
	IsBinary bool `json:"is_binary"`
	
	// Hunks for text files
	Hunks []Hunk `json:"hunks,omitempty"`
	
	// Statistics for this file
	Stats FileStats `json:"stats"`
}

// ChangeType represents the type of change to a file
type ChangeType string

const (
	ChangeTypeAdd    ChangeType = "add"
	ChangeTypeDelete ChangeType = "delete"
	ChangeTypeModify ChangeType = "modify"
	ChangeTypeRename ChangeType = "rename"
	ChangeTypeCopy   ChangeType = "copy"
)

// Hunk represents a contiguous section of changes
type Hunk struct {
	// Original file location
	OldStart int `json:"old_start"`
	OldLines int `json:"old_lines"`
	
	// Modified file location
	NewStart int `json:"new_start"`
	NewLines int `json:"new_lines"`
	
	// Optional section header
	Header string `json:"header,omitempty"`
	
	// The actual diff lines
	Lines []DiffLine `json:"lines"`
}

// DiffLine represents a single line in a diff
type DiffLine struct {
	// Line type (context, add, delete)
	Type LineType `json:"type"`
	
	// Line content (without prefix)
	Content string `json:"content"`
	
	// Original line number (for context and delete lines)
	OldNumber *int `json:"old_number,omitempty"`
	
	// New line number (for context and add lines)
	NewNumber *int `json:"new_number,omitempty"`
}

// LineType represents the type of a diff line
type LineType string

const (
	LineTypeContext LineType = "context"
	LineTypeAdd     LineType = "add"
	LineTypeDelete  LineType = "delete"
	LineTypeNoNewline LineType = "no_newline"
)

// FileStats contains statistics for a single file
type FileStats struct {
	LinesAdded   int `json:"lines_added"`
	LinesDeleted int `json:"lines_deleted"`
	LinesChanged int `json:"lines_changed"`
}

// ParseRequest represents a request to parse a diff
type ParseRequest struct {
	// The diff content to parse
	Content string `json:"content"`
	
	// Format of the input diff
	Format DiffFormat `json:"format,omitempty"`
	
	// Validation options
	Validate bool `json:"validate,omitempty"`
	
	// Security check options
	SecurityCheck bool `json:"security_check,omitempty"`
}

// ParseResponse represents a parsed diff
type ParseResponse struct {
	// Parsed file changes
	Changes []FileChange `json:"changes"`
	
	// Statistics
	Stats DiffStats `json:"stats"`
	
	// Any parsing warnings
	Warnings []string `json:"warnings,omitempty"`
	
	// Validation results
	Validation *ValidationResult `json:"validation,omitempty"`
}

// ValidationResult contains diff validation results
type ValidationResult struct {
	// Whether the diff is valid
	Valid bool `json:"valid"`
	
	// Validation errors
	Errors []ValidationError `json:"errors,omitempty"`
	
	// Security issues found
	SecurityIssues []SecurityIssue `json:"security_issues,omitempty"`
}

// ValidationError represents a validation error in a diff
type ValidationError struct {
	// Error type
	Type string `json:"type"`
	
	// Error message
	Message string `json:"message"`
	
	// Location in the diff
	Line int `json:"line,omitempty"`
	File string `json:"file,omitempty"`
}

// SecurityIssue represents a security concern in a diff
type SecurityIssue struct {
	// Issue severity
	Severity SecuritySeverity `json:"severity"`
	
	// Issue type
	Type string `json:"type"`
	
	// Description
	Description string `json:"description"`
	
	// Affected file
	File string `json:"file,omitempty"`
	
	// Affected line
	Line int `json:"line,omitempty"`
}

// SecuritySeverity represents the severity of a security issue
type SecuritySeverity string

const (
	SeverityLow    SecuritySeverity = "low"
	SeverityMedium SecuritySeverity = "medium"
	SeverityHigh   SecuritySeverity = "high"
	SeverityCritical SecuritySeverity = "critical"
)

// ApplyRequest represents a request to apply a diff
type ApplyRequest struct {
	// The diff to apply
	Diff string `json:"diff"`
	
	// Target content to apply the diff to
	Target string `json:"target"`
	
	// Options for application
	Options ApplyOptions `json:"options,omitempty"`
}

// ApplyOptions configures diff application
type ApplyOptions struct {
	// Allow fuzzy matching for line numbers
	Fuzzy bool `json:"fuzzy,omitempty"`
	
	// Maximum fuzz factor (lines to search)
	FuzzFactor int `json:"fuzz_factor,omitempty"`
	
	// Reverse the diff (unapply)
	Reverse bool `json:"reverse,omitempty"`
	
	// Strict mode (fail on any mismatch)
	Strict bool `json:"strict,omitempty"`
	
	// Create backup before applying
	CreateBackup bool `json:"create_backup,omitempty"`
}

// ApplyResponse represents the result of applying a diff
type ApplyResponse struct {
	// The result after applying the diff
	Result string `json:"result"`
	
	// Whether application was successful
	Success bool `json:"success"`
	
	// Applied hunks
	AppliedHunks int `json:"applied_hunks"`
	
	// Failed hunks
	FailedHunks int `json:"failed_hunks"`
	
	// Conflicts encountered
	Conflicts []Conflict `json:"conflicts,omitempty"`
	
	// Application report
	Report ApplyReport `json:"report"`
}

// Conflict represents a merge conflict during diff application
type Conflict struct {
	// File where conflict occurred
	File string `json:"file"`
	
	// Hunk that caused the conflict
	HunkIndex int `json:"hunk_index"`
	
	// Reason for conflict
	Reason string `json:"reason"`
	
	// Expected content
	Expected string `json:"expected"`
	
	// Actual content found
	Actual string `json:"actual"`
}

// ApplyReport contains detailed information about diff application
type ApplyReport struct {
	// Files processed
	FilesProcessed int `json:"files_processed"`
	
	// Files modified
	FilesModified int `json:"files_modified"`
	
	// Files skipped
	FilesSkipped int `json:"files_skipped"`
	
	// Total hunks
	TotalHunks int `json:"total_hunks"`
	
	// Processing time
	ProcessingTime time.Duration `json:"processing_time"`
	
	// Warnings
	Warnings []string `json:"warnings,omitempty"`
}

// Error types

// ErrInvalidDiff indicates an invalid diff format
type ErrInvalidDiff struct {
	Message string
	Line    int
}

func (e *ErrInvalidDiff) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("invalid diff at line %d: %s", e.Line, e.Message)
	}
	return fmt.Sprintf("invalid diff: %s", e.Message)
}

// ErrApplyFailed indicates diff application failure
type ErrApplyFailed struct {
	File   string
	Hunk   int
	Reason string
}

func (e *ErrApplyFailed) Error() string {
	return fmt.Sprintf("failed to apply diff to %s (hunk %d): %s", e.File, e.Hunk, e.Reason)
}

// ErrSecurityViolation indicates a security issue in the diff
type ErrSecurityViolation struct {
	Type        string
	Description string
	Severity    SecuritySeverity
}

func (e *ErrSecurityViolation) Error() string {
	return fmt.Sprintf("security violation (%s): %s [%s]", e.Severity, e.Description, e.Type)
}
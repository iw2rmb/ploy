package diff

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Formatter formats diffs for different output types
type Formatter struct {
	colorize bool
}

// NewFormatter creates a new diff formatter
func NewFormatter() *Formatter {
	return &Formatter{
		colorize: false, // Can be configured later
	}
}

// Format formats a diff response in the specified format
func (f *Formatter) Format(response *DiffResponse, format DiffFormat) (string, error) {
	switch format {
	case FormatUnified:
		return f.formatUnified(response), nil
	case FormatJSON:
		return f.formatJSON(response)
	case FormatSummary:
		return f.formatSummary(response), nil
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
}

// formatUnified formats as unified diff
func (f *Formatter) formatUnified(response *DiffResponse) string {
	// If content is already in unified format, return it
	if response.Format == FormatUnified && response.Content != "" {
		return response.Content
	}
	
	var builder strings.Builder
	
	for _, change := range response.Changes {
		// File header
		builder.WriteString(fmt.Sprintf("--- %s\n", change.OriginalPath))
		builder.WriteString(fmt.Sprintf("+++ %s\n", change.ModifiedPath))
		
		// Hunks
		for _, hunk := range change.Hunks {
			f.formatHunk(&builder, hunk)
		}
	}
	
	return builder.String()
}

// formatHunk formats a single hunk
func (f *Formatter) formatHunk(builder *strings.Builder, hunk Hunk) {
	// Hunk header
	builder.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@",
		hunk.OldStart, hunk.OldLines,
		hunk.NewStart, hunk.NewLines))
	
	if hunk.Header != "" {
		builder.WriteString(" " + hunk.Header)
	}
	builder.WriteString("\n")
	
	// Hunk lines
	for _, line := range hunk.Lines {
		f.formatLine(builder, line)
	}
}

// formatLine formats a single diff line
func (f *Formatter) formatLine(builder *strings.Builder, line DiffLine) {
	switch line.Type {
	case LineTypeContext:
		builder.WriteString(" " + line.Content + "\n")
	case LineTypeAdd:
		if f.colorize {
			builder.WriteString("\033[32m+" + line.Content + "\033[0m\n")
		} else {
			builder.WriteString("+" + line.Content + "\n")
		}
	case LineTypeDelete:
		if f.colorize {
			builder.WriteString("\033[31m-" + line.Content + "\033[0m\n")
		} else {
			builder.WriteString("-" + line.Content + "\n")
		}
	case LineTypeNoNewline:
		builder.WriteString("\\ No newline at end of file\n")
	}
}

// formatJSON formats as JSON
func (f *Formatter) formatJSON(response *DiffResponse) (string, error) {
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(data), nil
}

// formatSummary formats as human-readable summary
func (f *Formatter) formatSummary(response *DiffResponse) string {
	var builder strings.Builder
	
	// Overall statistics
	if response.Stats != nil {
		builder.WriteString("=== Diff Summary ===\n")
		builder.WriteString(fmt.Sprintf("Files changed: %d\n", response.Stats.FilesChanged))
		if response.Stats.FilesAdded > 0 {
			builder.WriteString(fmt.Sprintf("Files added: %d\n", response.Stats.FilesAdded))
		}
		if response.Stats.FilesDeleted > 0 {
			builder.WriteString(fmt.Sprintf("Files deleted: %d\n", response.Stats.FilesDeleted))
		}
		builder.WriteString(fmt.Sprintf("Lines added: %d\n", response.Stats.LinesAdded))
		builder.WriteString(fmt.Sprintf("Lines deleted: %d\n", response.Stats.LinesDeleted))
		builder.WriteString("\n")
	}
	
	// Per-file summary
	for i, change := range response.Changes {
		builder.WriteString(fmt.Sprintf("File %d: %s\n", i+1, f.getFileDescription(change)))
		builder.WriteString(fmt.Sprintf("  Type: %s\n", change.ChangeType))
		builder.WriteString(fmt.Sprintf("  Hunks: %d\n", len(change.Hunks)))
		builder.WriteString(fmt.Sprintf("  Lines: +%d -%d\n", 
			change.Stats.LinesAdded, change.Stats.LinesDeleted))
		
		if change.IsBinary {
			builder.WriteString("  Binary file\n")
		}
		
		builder.WriteString("\n")
	}
	
	// Metadata
	if response.Metadata.ProcessingTime > 0 {
		builder.WriteString(fmt.Sprintf("Processing time: %v\n", response.Metadata.ProcessingTime))
	}
	
	if len(response.Metadata.Warnings) > 0 {
		builder.WriteString("\nWarnings:\n")
		for _, warning := range response.Metadata.Warnings {
			builder.WriteString("  - " + warning + "\n")
		}
	}
	
	return builder.String()
}

// getFileDescription returns a description of file change
func (f *Formatter) getFileDescription(change FileChange) string {
	if change.OriginalPath == change.ModifiedPath {
		return change.OriginalPath
	}
	return fmt.Sprintf("%s → %s", change.OriginalPath, change.ModifiedPath)
}

// FormatApplyReport formats an apply report
func (f *Formatter) FormatApplyReport(report ApplyReport) string {
	var builder strings.Builder
	
	builder.WriteString("=== Apply Report ===\n")
	builder.WriteString(fmt.Sprintf("Files processed: %d\n", report.FilesProcessed))
	builder.WriteString(fmt.Sprintf("Files modified: %d\n", report.FilesModified))
	if report.FilesSkipped > 0 {
		builder.WriteString(fmt.Sprintf("Files skipped: %d\n", report.FilesSkipped))
	}
	builder.WriteString(fmt.Sprintf("Total hunks: %d\n", report.TotalHunks))
	
	if report.ProcessingTime > 0 {
		builder.WriteString(fmt.Sprintf("Processing time: %v\n", report.ProcessingTime))
	}
	
	if len(report.Warnings) > 0 {
		builder.WriteString("\nWarnings:\n")
		for _, warning := range report.Warnings {
			builder.WriteString("  - " + warning + "\n")
		}
	}
	
	return builder.String()
}

// FormatConflicts formats conflict information
func (f *Formatter) FormatConflicts(conflicts []Conflict) string {
	if len(conflicts) == 0 {
		return "No conflicts"
	}
	
	var builder strings.Builder
	builder.WriteString("=== Conflicts ===\n")
	
	for i, conflict := range conflicts {
		builder.WriteString(fmt.Sprintf("\nConflict %d:\n", i+1))
		builder.WriteString(fmt.Sprintf("  File: %s\n", conflict.File))
		builder.WriteString(fmt.Sprintf("  Hunk: %d\n", conflict.HunkIndex))
		builder.WriteString(fmt.Sprintf("  Reason: %s\n", conflict.Reason))
		
		if conflict.Expected != "" && conflict.Actual != "" {
			builder.WriteString("\n  Expected:\n")
			for _, line := range strings.Split(conflict.Expected, "\n") {
				if line != "" {
					builder.WriteString("    " + line + "\n")
				}
			}
			builder.WriteString("\n  Actual:\n")
			for _, line := range strings.Split(conflict.Actual, "\n") {
				if line != "" {
					builder.WriteString("    " + line + "\n")
				}
			}
		}
	}
	
	return builder.String()
}

// FormatValidation formats validation results
func (f *Formatter) FormatValidation(validation *ValidationResult) string {
	if validation == nil {
		return ""
	}
	
	var builder strings.Builder
	
	if validation.Valid {
		builder.WriteString("✓ Diff is valid\n")
	} else {
		builder.WriteString("✗ Diff validation failed\n")
	}
	
	if len(validation.Errors) > 0 {
		builder.WriteString("\nValidation Errors:\n")
		for _, err := range validation.Errors {
			builder.WriteString(fmt.Sprintf("  - %s: %s", err.Type, err.Message))
			if err.File != "" {
				builder.WriteString(fmt.Sprintf(" (file: %s)", err.File))
			}
			if err.Line > 0 {
				builder.WriteString(fmt.Sprintf(" (line: %d)", err.Line))
			}
			builder.WriteString("\n")
		}
	}
	
	if len(validation.SecurityIssues) > 0 {
		builder.WriteString("\nSecurity Issues:\n")
		for _, issue := range validation.SecurityIssues {
			icon := f.getSeverityIcon(issue.Severity)
			builder.WriteString(fmt.Sprintf("  %s %s: %s\n", 
				icon, issue.Type, issue.Description))
			if issue.File != "" {
				builder.WriteString(fmt.Sprintf("      File: %s\n", issue.File))
			}
			if issue.Line > 0 {
				builder.WriteString(fmt.Sprintf("      Line: %d\n", issue.Line))
			}
		}
	}
	
	return builder.String()
}

// getSeverityIcon returns an icon for severity level
func (f *Formatter) getSeverityIcon(severity SecuritySeverity) string {
	switch severity {
	case SeverityCritical:
		return "🔴"
	case SeverityHigh:
		return "🟠"
	case SeverityMedium:
		return "🟡"
	case SeverityLow:
		return "🔵"
	default:
		return "⚪"
	}
}

// EnableColor enables color output
func (f *Formatter) EnableColor(enable bool) {
	f.colorize = enable
}

// FormatStatistics formats diff statistics in a compact form
func (f *Formatter) FormatStatistics(stats *DiffStats) string {
	if stats == nil {
		return ""
	}
	
	parts := []string{}
	
	if stats.FilesChanged > 0 {
		parts = append(parts, fmt.Sprintf("%d file%s changed",
			stats.FilesChanged, pluralize(stats.FilesChanged)))
	}
	
	if stats.LinesAdded > 0 {
		parts = append(parts, fmt.Sprintf("%d insertion%s(+)",
			stats.LinesAdded, pluralize(stats.LinesAdded)))
	}
	
	if stats.LinesDeleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deletion%s(-)",
			stats.LinesDeleted, pluralize(stats.LinesDeleted)))
	}
	
	return strings.Join(parts, ", ")
}

// Helper function for pluralization
func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
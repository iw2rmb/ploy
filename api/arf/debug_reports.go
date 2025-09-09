package arf

import (
	"fmt"
	"strings"
	"time"
)

// generateMarkdownReport creates a comprehensive markdown report for a transformation
func generateMarkdownReport(status *TransformationStatus, diff string) string {
	var report strings.Builder

	// Header
	report.WriteString(fmt.Sprintf("# Transformation Report: %s\n", status.TransformationID))
	report.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	// Summary section
	report.WriteString("## 📊 Summary\n")
	report.WriteString(fmt.Sprintf("- **Status**: %s\n", status.Status))

	if !status.StartTime.IsZero() && !status.EndTime.IsZero() {
		duration := status.EndTime.Sub(status.StartTime)
		report.WriteString(fmt.Sprintf("- **Duration**: %s\n", formatDuration(duration)))
	}

	report.WriteString(fmt.Sprintf("- **Workflow Stage**: %s\n", status.WorkflowStage))
	report.WriteString(fmt.Sprintf("- **Healing Attempts**: %d\n", len(status.Children)))
	report.WriteString("\n")

	// Timeline section
	report.WriteString("## ⏱️ Timeline\n")
	timeline := buildTimeline(status)
	report.WriteString(formatTimelineMarkdown(timeline))
	report.WriteString("\n")

	// Healing Attempts section
	if len(status.Children) > 0 {
		report.WriteString("## 🔄 Healing Attempts\n")
		report.WriteString(formatHealingAttemptsMarkdown(status.Children))
		report.WriteString("\n")
	}

	// Code Changes section
	report.WriteString("## 📝 Code Changes\n")
	report.WriteString(formatCodeChangesMarkdown(status.Children, diff))
	report.WriteString("\n")

	// Cost Analysis section
	if status.CoordinatorMetrics != nil {
		report.WriteString("## 💰 Cost Analysis\n")
		report.WriteString(formatCostAnalysisMarkdown(status.CoordinatorMetrics))
		report.WriteString("\n")
	}

	return report.String()
}

// formatTimelineMarkdown formats timeline data as markdown
func formatTimelineMarkdown(timeline *TransformationTimeline) string {
	var md strings.Builder

	md.WriteString("### Step-by-Step Execution\n")

	for _, entry := range timeline.Timeline {
		duration := ""
		if entry.Duration != nil && *entry.Duration > 0 {
			duration = fmt.Sprintf(" [%s]", formatDuration(*entry.Duration))
		}

		md.WriteString(fmt.Sprintf("- **%s**%s **%s** - %s\n",
			entry.Timestamp.Format("15:04:05"), duration, entry.EventType, entry.Status))

		if entry.Description != "" {
			md.WriteString(fmt.Sprintf("  - %s\n", entry.Description))
		}
	}

	return md.String()
}

// formatHealingAttemptsMarkdown formats healing attempts as markdown
func formatHealingAttemptsMarkdown(attempts []HealingAttempt) string {
	var md strings.Builder

	var formatAttempt func([]HealingAttempt, int)
	formatAttempt = func(attempts []HealingAttempt, depth int) {
		for _, attempt := range attempts {
			indent := strings.Repeat("  ", depth)

			md.WriteString(fmt.Sprintf("%s- **Path**: %s\n", indent, attempt.AttemptPath))
			md.WriteString(fmt.Sprintf("%s  - **Trigger**: %s\n", indent, attempt.TriggerReason))
			md.WriteString(fmt.Sprintf("%s  - **Status**: %s\n", indent, attempt.Status))

			if attempt.LLMAnalysis != nil {
				md.WriteString(fmt.Sprintf("%s  - **LLM Analysis**: %.0f%% confidence - %s\n",
					indent, attempt.LLMAnalysis.Confidence*100, attempt.LLMAnalysis.SuggestedFix))
			}

			if len(attempt.TargetErrors) > 0 {
				md.WriteString(fmt.Sprintf("%s  - **Target Errors**: %s\n", indent,
					strings.Join(attempt.TargetErrors, ", ")))
			}

			if len(attempt.Children) > 0 {
				formatAttempt(attempt.Children, depth+1)
			}

			md.WriteString("\n")
		}
	}

	formatAttempt(attempts, 0)
	return md.String()
}

// formatCodeChangesMarkdown formats code changes as markdown
func formatCodeChangesMarkdown(attempts []HealingAttempt, diff string) string {
	var md strings.Builder

	// If we have a diff from OpenRewrite, display it
	if diff != "" {
		md.WriteString("### Transformation Diff\n")
		md.WriteString("```diff\n")

		// Limit diff output to reasonable size for report
		lines := strings.Split(diff, "\n")
		maxLines := 100
		if len(lines) > maxLines {
			md.WriteString(strings.Join(lines[:maxLines], "\n"))
			md.WriteString("\n... (truncated, showing first 100 lines)\n")
		} else {
			md.WriteString(diff)
		}

		md.WriteString("\n```\n\n")

		// Extract file list from diff
		md.WriteString("### Files Modified\n")
		filesModified := extractFilesFromDiff(diff)
		if len(filesModified) > 0 {
			for _, file := range filesModified {
				md.WriteString(fmt.Sprintf("- %s\n", file))
			}
		} else {
			md.WriteString("- Changes detected in transformation\n")
		}
	} else {
		// Fall back to healing attempt changes if no diff
		md.WriteString("### Files Modified\n")

		var hasChanges bool
		var collectChanges func([]HealingAttempt)
		collectChanges = func(attempts []HealingAttempt) {
			for _, attempt := range attempts {
				if attempt.LLMAnalysis != nil && attempt.LLMAnalysis.SuggestedFix != "" {
					md.WriteString(fmt.Sprintf("- **Change**: %s\n", attempt.LLMAnalysis.SuggestedFix))
					hasChanges = true
				}
				collectChanges(attempt.Children)
			}
		}

		collectChanges(attempts)

		if !hasChanges {
			md.WriteString("- No detailed file changes recorded\n")
		}
	}

	return md.String()
}

// extractFilesFromDiff parses a unified diff to extract modified file names
func extractFilesFromDiff(diff string) []string {
	var files []string
	seenFiles := make(map[string]bool)

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		// Look for diff headers like "diff --git a/file.java b/file.java"
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				// Extract filename from a/filename format
				file := strings.TrimPrefix(parts[2], "a/")
				if !seenFiles[file] {
					files = append(files, file)
					seenFiles[file] = true
				}
			}
		}
		// Also look for +++ and --- lines
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[1] != "/dev/null" {
				file := strings.TrimPrefix(parts[1], "b/")
				file = strings.TrimPrefix(file, "a/")
				if !seenFiles[file] && file != "" {
					files = append(files, file)
					seenFiles[file] = true
				}
			}
		}
	}

	return files
}

// formatCostAnalysisMarkdown formats cost analysis as markdown
func formatCostAnalysisMarkdown(metrics *HealingCoordinatorMetrics) string {
	var md strings.Builder

	md.WriteString(fmt.Sprintf("- **Total LLM calls**: %d\n", metrics.TotalLLMCalls))
	md.WriteString(fmt.Sprintf("- **Total tokens**: %d\n", metrics.TotalLLMTokens))
	md.WriteString(fmt.Sprintf("- **Estimated cost**: $%.2f\n", metrics.TotalLLMCost))

	if metrics.LLMCacheHitRate > 0 {
		md.WriteString(fmt.Sprintf("- **Cache hit rate**: %.1f%%\n", metrics.LLMCacheHitRate*100))
	}

	return md.String()
}

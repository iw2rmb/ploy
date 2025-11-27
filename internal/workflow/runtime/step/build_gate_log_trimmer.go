package step

import (
	"strings"
)

// TrimBuildGateLog returns a trimmed view of build gate logs for known tools.
// For Maven and Gradle, it keeps the most relevant failure region (stack trace
// and summary) and drops earlier noise such as plugin startup banners or task
// noise. For unknown tools, it returns the original logText unchanged.
func TrimBuildGateLog(tool, logText string) string {
	tool = strings.ToLower(strings.TrimSpace(tool))
	switch tool {
	case "maven":
		return trimMavenLog(logText)
	case "gradle":
		return trimGradleLog(logText)
	default:
		return logText
	}
}

func trimMavenLog(logText string) string {
	lines := strings.Split(logText, "\n")
	if len(lines) == 0 {
		return logText
	}

	// Prefer the first "[ERROR] Tests run:" summary as the anchor; fall back
	// to the first "[ERROR]" line when the summary is missing.
	anchor := -1
	for i, l := range lines {
		if strings.Contains(l, "[ERROR] Tests run:") {
			anchor = i
			break
		}
	}
	if anchor == -1 {
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "[ERROR]") {
				anchor = i
				break
			}
		}
	}
	if anchor == -1 {
		return logText
	}

	// Keep some context above the anchor so the stack trace leading into the
	// error summary is preserved.
	const contextLines = 40
	start := anchor - contextLines
	if start < 0 {
		start = 0
	}

	end := len(lines)
	for i := anchor + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[INFO] BUILD") ||
			strings.HasPrefix(trimmed, "[INFO] Total time:") ||
			strings.HasPrefix(trimmed, "[INFO] Finished at:") ||
			strings.HasPrefix(trimmed, "[INFO] ------------------------------------------------------------------------") {
			end = i
			break
		}
	}
	if end <= start {
		return logText
	}

	result := strings.Join(lines[start:end], "\n")
	// Preserve trailing newline when present in the original text to avoid
	// surprising callers that rely on newline-terminated logs.
	if strings.HasSuffix(logText, "\n") {
		result += "\n"
	}
	return result
}

func trimGradleLog(logText string) string {
	lines := strings.Split(logText, "\n")
	if len(lines) == 0 {
		return logText
	}

	// Anchor on the standard Gradle failure header.
	anchor := -1
	for i, l := range lines {
		if strings.Contains(l, "FAILURE: Build failed with an exception.") {
			anchor = i
			break
		}
	}
	if anchor == -1 {
		return logText
	}

	const contextLines = 20
	start := anchor - contextLines
	if start < 0 {
		start = 0
	}

	end := len(lines)
	for i := anchor + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		// Keep the "BUILD FAILED" summary, but drop anything after it.
		if strings.HasPrefix(trimmed, "BUILD FAILED in ") || strings.HasPrefix(trimmed, "BUILD FAILED") {
			end = i + 1
			break
		}
	}
	if end <= start {
		return logText
	}

	result := strings.Join(lines[start:end], "\n")
	if strings.HasSuffix(logText, "\n") {
		result += "\n"
	}
	return result
}

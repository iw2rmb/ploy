package step

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	gradleJavaIssueStartRe   = regexp.MustCompile(`^\/workspace\/.*?\.java:[0-9]{1,}: error: .*`)
	gradleKotlinIssueStartRe = regexp.MustCompile(`^e: \/workspace\/.*?\.kt:[0-9]{1,}:[0-9]{1,} .*`)
	gradleScalaIssueStartRe  = regexp.MustCompile(`^\[error\] \/workspace\/.*?\.scala:[0-9]{1,}: .*`)
	gradleKaptIssueStartRe   = regexp.MustCompile(`^e: \[kapt\] .*`)
	gradleErrorsCountRe      = regexp.MustCompile(`^([0-9]{1,}) errors$`)
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
	// Anchor on the first "[ERROR]" line and keep everything from there to the
	// end of the log. With Maven --ff enabled this typically corresponds to the
	// first meaningful failure block (compilation error or test failure summary).
	anchor := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "[ERROR] ") {
			anchor = i
			break
		}
	}
	if anchor == -1 {
		return logText
	}

	result := strings.Join(lines[anchor:], "\n")
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

	splitAt := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "* What went wrong:" {
			splitAt = i
			break
		}
	}
	if splitAt == -1 {
		return logText
	}

	firstPart := lines[:splitAt]
	secondPart := lines[splitAt:]

	compilerIssues := collectGradleCompilerIssues(firstPart)
	reconcileGradleCompilerIssuesByErrorCount(firstPart, &compilerIssues)

	secondPart = removeGradleTryBlock(secondPart)
	secondPart = trimGradleStacktraceOccurrences(secondPart)

	out := make([]string, 0, len(secondPart)+len(compilerIssues)*4)
	for _, issue := range compilerIssues {
		out = append(out, issue...)
	}
	out = append(out, secondPart...)

	result := strings.Join(out, "\n")
	if strings.HasSuffix(logText, "\n") {
		result += "\n"
	}
	return result
}

func collectGradleCompilerIssues(lines []string) [][]string {
	issues := make([][]string, 0)
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case gradleJavaIssueStartRe.MatchString(line):
			block, next := collectContinuation(lines, i, func(s string) bool { return strings.HasPrefix(s, " ") })
			issues = append(issues, block)
			i = next
		case gradleKotlinIssueStartRe.MatchString(line):
			block, next := collectContinuation(lines, i, func(s string) bool { return strings.HasPrefix(s, " ") })
			issues = append(issues, block)
			i = next
		case gradleScalaIssueStartRe.MatchString(line):
			block, next := collectContinuation(lines, i, func(s string) bool { return strings.HasPrefix(s, "[error]  ") })
			issues = append(issues, block)
			i = next
		case gradleKaptIssueStartRe.MatchString(line):
			block, next := collectContinuation(lines, i, func(s string) bool { return strings.HasPrefix(s, " ") })
			issues = append(issues, block)
			i = next
		}
	}
	return issues
}

func collectContinuation(lines []string, start int, cont func(string) bool) ([]string, int) {
	block := []string{lines[start]}
	i := start + 1
	for i < len(lines) && cont(lines[i]) {
		block = append(block, lines[i])
		i++
	}
	return block, i - 1
}

func reconcileGradleCompilerIssuesByErrorCount(firstPart []string, issues *[][]string) {
	if len(*issues) == 0 || len(firstPart) == 0 {
		return
	}
	start := len(firstPart) - 3
	if start < 0 {
		start = 0
	}
	for i := len(firstPart) - 1; i >= start; i-- {
		match := gradleErrorsCountRe.FindStringSubmatch(strings.TrimSpace(firstPart[i]))
		if len(match) != 2 {
			continue
		}
		k, err := strconv.Atoi(match[1])
		if err != nil || k < 0 {
			return
		}
		if len(*issues) > k {
			*issues = (*issues)[len(*issues)-k:]
		}
		return
	}
}

func removeGradleTryBlock(lines []string) []string {
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "* Try:" {
			start = i
			break
		}
	}
	if start == -1 {
		return lines
	}
	end := start + 1
	for end < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[end]), "> ") {
		end++
	}
	return append(slicesClone(lines[:start]), lines[end:]...)
}

func trimGradleStacktraceOccurrences(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(lines[i], "  at") {
			out = append(out, lines[i])
			i++
			continue
		}

		j := i
		for j < len(lines) && (strings.HasPrefix(lines[j], "  at") || strings.HasPrefix(lines[j], "Caused by")) {
			j++
		}
		out = append(out, dedupeGradleStacktraceBlock(lines[i:j])...)
		i = j
	}
	return out
}

func dedupeGradleStacktraceBlock(block []string) []string {
	if len(block) == 0 {
		return block
	}

	parts := splitByCausedBy(block)
	if len(parts) <= 1 {
		return block
	}

	referenceFrames := make(map[string]struct{}, len(parts[0]))
	for _, line := range parts[0] {
		if strings.HasPrefix(line, "  at") {
			referenceFrames[line] = struct{}{}
		}
	}
	if len(referenceFrames) == 0 {
		return block
	}

	for i := 1; i < len(parts); i++ {
		cutAt := -1
		for idx, line := range parts[i] {
			if !strings.HasPrefix(line, "  at") {
				continue
			}
			if _, ok := referenceFrames[line]; ok {
				cutAt = idx
				break
			}
		}
		if cutAt == -1 {
			continue
		}

		removedFrames := 0
		for _, line := range parts[i][cutAt:] {
			if strings.HasPrefix(line, "  at") {
				removedFrames++
			}
		}
		if removedFrames == 0 {
			continue
		}
		parts[i] = append(parts[i][:cutAt], fmt.Sprintf("...repeated %d frames", removedFrames))
	}

	out := make([]string, 0, len(block))
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func splitByCausedBy(block []string) [][]string {
	parts := make([][]string, 0, 2)
	current := make([]string, 0, len(block))
	for _, line := range block {
		if strings.HasPrefix(line, "Caused by") && len(current) > 0 {
			parts = append(parts, current)
			current = []string{line}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		parts = append(parts, current)
	}
	return parts
}

func slicesClone(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	copy(out, lines)
	return out
}

package step

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	gradleJavaIssueStartRe   = regexp.MustCompile(`^\/workspace\/.*?\.java:[0-9]{1,}: error: .*`)
	gradleKotlinIssueStartRe = regexp.MustCompile(`^e: \/workspace\/.*?\.kt:[0-9]{1,}:[0-9]{1,} .*`)
	gradleScalaIssueStartRe  = regexp.MustCompile(`^\[error\] \/workspace\/.*?\.scala:[0-9]{1,}: .*`)
	gradleKaptIssueStartRe   = regexp.MustCompile(`^e: \[kapt\] .*`)
	gradleErrorsCountRe      = regexp.MustCompile(`^([0-9]{1,}) errors$`)
	gradleJavaErrorLineRe    = regexp.MustCompile(`^(\/workspace\/.*?\.java):([0-9]{1,}): error: (.+)$`)
	gradlePluginRequestRe    = regexp.MustCompile(`An exception occurred applying plugin request \[id: '([^']+)'(?:, version: '([^']+)')?\]`)
)

type gradleStructuredPayload struct {
	Mode          string                  `yaml:"mode"`
	Task          string                  `yaml:"task,omitempty"`
	PluginID      string                  `yaml:"plugin_id,omitempty"`
	PluginVersion string                  `yaml:"plugin_version,omitempty"`
	Errors        []gradleStructuredError `yaml:"errors"`
}

type gradleStructuredError struct {
	Message   string                    `yaml:"message"`
	Signature string                    `yaml:"signature,omitempty"`
	Symbol    string                    `yaml:"symbol,omitempty"`
	Location  string                    `yaml:"location,omitempty"`
	Files     []gradleStructuredFileRef `yaml:"files,omitempty"`
}

type gradleStructuredFileRef struct {
	Path    string `yaml:"path"`
	Line    int    `yaml:"line,omitempty"`
	Snippet string `yaml:"snippet,omitempty"`
}

// TrimBuildGateLog returns a trimmed view of build gate logs for known tools.
// For Maven and Gradle, it keeps the most relevant failure region (stack trace
// and summary) and drops earlier noise such as plugin startup banners or task
// noise. For unknown tools, it returns the original logText unchanged.
func TrimBuildGateLog(tool, logText string) string {
	trimmed, _ := BuildGateLogFindingContent(tool, logText)
	return trimmed
}

// BuildGateLogFindingContent returns the human-focused trimmed message and an
// optional structured evidence payload for known build tools.
func BuildGateLogFindingContent(tool, logText string) (string, string) {
	tool = strings.ToLower(strings.TrimSpace(tool))
	switch tool {
	case "maven":
		return trimMavenLog(logText), ""
	case "gradle":
		return trimGradleLogAndEvidence(logText)
	default:
		return logText, ""
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
	trimmed, _ := trimGradleLogAndEvidence(logText)
	return trimmed
}

func trimGradleLogAndEvidence(logText string) (string, string) {
	lines := strings.Split(logText, "\n")
	if len(lines) == 0 {
		return logText, ""
	}

	splitAt := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "* What went wrong:" {
			splitAt = i
			break
		}
	}
	if splitAt == -1 {
		return logText, ""
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
	return result, renderGradleStructuredEvidence(firstPart, secondPart, result)
}

func renderGradleStructuredEvidence(firstPart, secondPart []string, trimmed string) string {
	if payload, ok := buildGradleCompileJavaPayload(firstPart, secondPart); ok {
		return marshalGradleStructuredPayload(payload)
	}
	if payload, ok := buildGradlePluginApplyPayload(secondPart); ok {
		return marshalGradleStructuredPayload(payload)
	}
	raw := strings.TrimSpace(extractGradleWhatWentWrong(secondPart))
	if raw == "" {
		raw = normalizeMultilineLimit(trimmed, 20)
	}
	if raw == "" {
		return ""
	}
	return marshalGradleStructuredPayload(gradleStructuredPayload{
		Mode: "raw",
		Errors: []gradleStructuredError{{
			Message: raw,
		}},
	})
}

func buildGradleCompileJavaPayload(firstPart, secondPart []string) (gradleStructuredPayload, bool) {
	if !strings.Contains(strings.Join(secondPart, "\n"), "Execution failed for task ':compileJava'.") {
		return gradleStructuredPayload{}, false
	}

	issues := collectGradleCompilerIssues(firstPart)
	reconcileGradleCompilerIssuesByErrorCount(firstPart, &issues)
	if len(issues) == 0 {
		return gradleStructuredPayload{}, false
	}

	groupBySig := map[string]*gradleStructuredError{}
	order := make([]string, 0, len(issues))
	for _, issue := range issues {
		errEntry, fileRef, ok := parseGradleJavaIssueBlock(issue)
		if !ok {
			continue
		}
		current, exists := groupBySig[errEntry.Signature]
		if !exists {
			entry := errEntry
			entry.Files = nil
			groupBySig[errEntry.Signature] = &entry
			order = append(order, errEntry.Signature)
			current = &entry
		}
		current.Files = append(current.Files, fileRef)
		groupBySig[errEntry.Signature] = current
	}

	if len(groupBySig) == 0 {
		return gradleStructuredPayload{}, false
	}

	errorsOut := make([]gradleStructuredError, 0, len(order))
	for _, sig := range order {
		group := groupBySig[sig]
		seen := make(map[string]struct{}, len(group.Files))
		files := make([]gradleStructuredFileRef, 0, len(group.Files))
		for _, f := range group.Files {
			key := fmt.Sprintf("%s:%d:%s", f.Path, f.Line, f.Snippet)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			files = append(files, f)
		}
		sort.SliceStable(files, func(i, j int) bool {
			if files[i].Path == files[j].Path {
				return files[i].Line < files[j].Line
			}
			return files[i].Path < files[j].Path
		})
		group.Files = files
		errorsOut = append(errorsOut, *group)
	}

	return gradleStructuredPayload{
		Mode:   "compile_java",
		Task:   "compileJava",
		Errors: errorsOut,
	}, true
}

func parseGradleJavaIssueBlock(block []string) (gradleStructuredError, gradleStructuredFileRef, bool) {
	if len(block) == 0 {
		return gradleStructuredError{}, gradleStructuredFileRef{}, false
	}
	m := gradleJavaErrorLineRe.FindStringSubmatch(strings.TrimSpace(block[0]))
	if len(m) != 4 {
		return gradleStructuredError{}, gradleStructuredFileRef{}, false
	}
	line, err := strconv.Atoi(m[2])
	if err != nil {
		return gradleStructuredError{}, gradleStructuredFileRef{}, false
	}

	message := strings.TrimSpace(m[3])
	symbol := ""
	location := ""
	snippet := ""
	for _, raw := range block[1:] {
		lineText := strings.TrimSpace(raw)
		if lineText == "" || lineText == "^" {
			continue
		}
		switch {
		case strings.HasPrefix(lineText, "symbol:"):
			symbol = strings.TrimSpace(strings.TrimPrefix(lineText, "symbol:"))
		case strings.HasPrefix(lineText, "location:"):
			location = strings.TrimSpace(strings.TrimPrefix(lineText, "location:"))
		case snippet == "":
			snippet = lineText
		}
	}

	sigParts := []string{message}
	if symbol != "" {
		sigParts = append(sigParts, "symbol="+symbol)
	}
	if location != "" {
		sigParts = append(sigParts, "location="+location)
	}
	signature := strings.Join(sigParts, " | ")

	return gradleStructuredError{
			Message:   message,
			Signature: signature,
			Symbol:    symbol,
			Location:  location,
		}, gradleStructuredFileRef{
			Path:    strings.TrimSpace(m[1]),
			Line:    line,
			Snippet: snippet,
		}, true
}

func buildGradlePluginApplyPayload(secondPart []string) (gradleStructuredPayload, bool) {
	if len(secondPart) == 0 {
		return gradleStructuredPayload{}, false
	}
	joined := strings.Join(secondPart, "\n")
	if !strings.Contains(joined, "An exception occurred applying plugin request") {
		return gradleStructuredPayload{}, false
	}

	pluginID := ""
	pluginVersion := ""
	if match := gradlePluginRequestRe.FindStringSubmatch(joined); len(match) >= 2 {
		pluginID = strings.TrimSpace(match[1])
		if len(match) >= 3 {
			pluginVersion = strings.TrimSpace(match[2])
		}
	}

	block := strings.TrimSpace(extractGradleWhatWentWrong(secondPart))
	if block == "" {
		block = normalizeMultilineLimit(joined, 20)
	}
	if block == "" {
		return gradleStructuredPayload{}, false
	}

	return gradleStructuredPayload{
		Mode:          "plugin_apply",
		PluginID:      pluginID,
		PluginVersion: pluginVersion,
		Errors: []gradleStructuredError{{
			Message: block,
		}},
	}, true
}

func extractGradleWhatWentWrong(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "* What went wrong:" {
			start = i + 1
			break
		}
	}
	if start == -1 || start >= len(lines) {
		return ""
	}
	collected := make([]string, 0, 8)
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "BUILD FAILED") {
			break
		}
		if trimmed == "" {
			continue
		}
		collected = append(collected, trimmed)
	}
	return strings.Join(collected, "\n")
}

func normalizeMultilineLimit(text string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	out := make([]string, 0, maxLines)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
		if len(out) == maxLines {
			break
		}
	}
	return strings.Join(out, "\n")
}

func marshalGradleStructuredPayload(payload gradleStructuredPayload) string {
	if len(payload.Errors) == 0 {
		return ""
	}
	data, err := yaml.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
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
		if !isGradleStackFrameLine(lines[i]) {
			out = append(out, lines[i])
			i++
			continue
		}

		j := i
		for j < len(lines) && (isGradleStackFrameLine(lines[j]) || isGradleCausedByLine(lines[j])) {
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
		if !isGradleStackFrameLine(line) {
			continue
		}
		referenceFrames[normalizeGradleStackFrame(line)] = struct{}{}
	}
	if len(referenceFrames) == 0 {
		return block
	}

	for i := 1; i < len(parts); i++ {
		cutAt := -1
		for idx, line := range parts[i] {
			if !isGradleStackFrameLine(line) {
				continue
			}
			if _, ok := referenceFrames[normalizeGradleStackFrame(line)]; ok {
				cutAt = idx
				break
			}
		}
		if cutAt == -1 {
			continue
		}

		removedFrames := 0
		for _, line := range parts[i][cutAt:] {
			if isGradleStackFrameLine(line) {
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
		if isGradleCausedByLine(line) && len(current) > 0 {
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

func isGradleStackFrameLine(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " \t"), "at ")
}

func isGradleCausedByLine(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " \t"), "Caused by")
}

func normalizeGradleStackFrame(line string) string {
	return strings.TrimLeft(line, " \t")
}

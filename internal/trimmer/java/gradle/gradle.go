package gradle

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	Tool            = "gradle"
	maxMessageLines = 200
	maxMessageBytes = 64 << 10
)

var (
	gradleJavaIssueStartRe         = regexp.MustCompile(`^\/workspace\/.*?\.java:[0-9]{1,}: error: .*`)
	gradleKotlinIssueStartRe       = regexp.MustCompile(`^e: (?:file:\/\/)?\/workspace\/.*?\.kt:[0-9]{1,}:[0-9]{1,} .*`)
	gradleScalaIssueStartRe        = regexp.MustCompile(`^\[error\] \/workspace\/.*?\.scala:[0-9]{1,}: .*`)
	gradleKaptIssueStartRe         = regexp.MustCompile(`^e: \[kapt\] .*`)
	gradleErrorsCountRe            = regexp.MustCompile(`^([0-9]{1,}) errors$`)
	gradleJavaErrorLineRe          = regexp.MustCompile(`^(\/workspace\/.*?\.java):([0-9]{1,}): error: (.+)$`)
	gradleKotlinErrorLineRe        = regexp.MustCompile(`^e: ((?:file:\/\/)?\/workspace\/.*?\.kt):([0-9]{1,}):([0-9]{1,}) (.+)$`)
	gradleCompileJavaTaskFailureRe = regexp.MustCompile(`Execution failed for task ':(?:[^']+:)*compileJava'\.`)
	gradleKotlinTaskFailureRe      = regexp.MustCompile(`Execution failed for task ':(?:[^']+:)*(compileKotlin|kaptKotlin)'\.`)
	gradleActionableTasksRe        = regexp.MustCompile(`^[0-9]+ actionable tasks?: .+$`)
	gradlePluginRequestRe          = regexp.MustCompile(`An exception occurred applying plugin request \[id: '([^']+)'(?:, version: '([^']+)')?\]`)
)

type Result struct {
	Tool     string    `json:"tool" yaml:"tool"`
	Message  string    `json:"message,omitempty" yaml:"message,omitempty"`
	Evidence *Evidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

type Evidence struct {
	Task   string  `json:"task,omitempty" yaml:"task,omitempty"`
	Errors []Error `json:"errors,omitempty" yaml:"errors,omitempty"`
}

type Error struct {
	Message  string   `json:"message" yaml:"message"`
	Plugin   string   `json:"plugin,omitempty" yaml:"plugin,omitempty"`
	Version  string   `json:"version,omitempty" yaml:"version,omitempty"`
	Symbol   string   `json:"symbol,omitempty" yaml:"symbol,omitempty"`
	Location string   `json:"location,omitempty" yaml:"location,omitempty"`
	Base     string   `json:"base,omitempty" yaml:"base,omitempty"`
	Snippet  string   `json:"snippet,omitempty" yaml:"snippet,omitempty"`
	Files    []string `json:"files,omitempty" yaml:"files,omitempty"`
}

type fileRef struct {
	Path    string
	Snippet string
}

type gradleErrorGroup struct {
	entry Error
	files []fileRef
}

func Trim(logText string) Result {
	message, evidence := trimGradleLogAndEvidence(logText)
	message = boundMessage(message)
	if hasCompleteCompilerEvidence(evidence) {
		message = ""
	}
	return Result{
		Tool:     Tool,
		Message:  message,
		Evidence: evidence,
	}
}

func hasCompleteCompilerEvidence(evidence *Evidence) bool {
	if evidence == nil || len(evidence.Errors) == 0 {
		return false
	}
	switch evidence.Task {
	case "compileJava", "compileKotlin", "kaptKotlin":
	default:
		return false
	}
	for _, errEntry := range evidence.Errors {
		if strings.TrimSpace(errEntry.Message) == "" || len(errEntry.Files) == 0 {
			return false
		}
		for _, file := range errEntry.Files {
			if strings.TrimSpace(file) == "" {
				return false
			}
		}
	}
	return true
}

func trimGradleLogAndEvidence(logText string) (string, *Evidence) {
	lines := strings.Split(logText, "\n")
	if len(lines) == 0 {
		return logText, nil
	}

	splitAt := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "* What went wrong:" {
			splitAt = i
			break
		}
	}
	if splitAt == -1 {
		return logText, nil
	}

	firstPart := lines[:splitAt]
	secondPart := lines[splitAt:]

	compilerIssues := collectGradleCompilerIssues(firstPart)
	reconcileGradleCompilerIssuesByErrorCount(firstPart, &compilerIssues)

	secondPart = removeGradleInterleavedNoiseLines(secondPart)
	secondPart = removeGradleTryBlock(secondPart)
	secondPart = removeGradleExceptionBlock(secondPart)
	if len(compilerIssues) == 0 {
		secondPart = trimGradleStacktraceOccurrences(secondPart)
	}
	secondPart = removeGradleDeprecationFooter(secondPart)
	secondPart = trimGradleAfterTerminalFailure(secondPart)

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

func renderGradleStructuredEvidence(firstPart, secondPart []string, trimmed string) *Evidence {
	if payload, ok := buildGradleCompileJavaPayload(firstPart, secondPart); ok {
		return &payload
	}
	if payload, ok := buildGradleKotlinPayload(firstPart, secondPart); ok {
		return &payload
	}
	if payload, ok := buildGradlePluginApplyPayload(secondPart); ok {
		return &payload
	}
	raw := strings.TrimSpace(extractGradleWhatWentWrong(secondPart))
	if raw == "" {
		raw = normalizeMultilineLimit(trimmed, 20)
	}
	if raw == "" {
		return nil
	}
	return &Evidence{
		Errors: []Error{{
			Message: raw,
		}},
	}
}

func buildGradleCompileJavaPayload(firstPart, secondPart []string) (Evidence, bool) {
	if !gradleCompileJavaTaskFailureRe.MatchString(strings.Join(secondPart, "\n")) {
		return Evidence{}, false
	}

	issues := collectGradleCompilerIssues(firstPart)
	reconcileGradleCompilerIssuesByErrorCount(firstPart, &issues)
	if len(issues) == 0 {
		return Evidence{}, false
	}

	groupBySig := map[string]*gradleErrorGroup{}
	order := make([]string, 0, len(issues))
	for _, issue := range issues {
		sigKey, errEntry, fileRef, ok := parseGradleJavaIssueBlock(issue)
		if !ok {
			continue
		}
		current, exists := groupBySig[sigKey]
		if !exists {
			groupBySig[sigKey] = &gradleErrorGroup{entry: errEntry}
			order = append(order, sigKey)
			current = groupBySig[sigKey]
		}
		current.files = append(current.files, fileRef)
	}

	if len(groupBySig) == 0 {
		return Evidence{}, false
	}

	return Evidence{
		Task:   "compileJava",
		Errors: renderGroupedGradleErrors(order, groupBySig),
	}, true
}

func buildGradleKotlinPayload(firstPart, secondPart []string) (Evidence, bool) {
	m := gradleKotlinTaskFailureRe.FindStringSubmatch(strings.Join(secondPart, "\n"))
	if len(m) != 2 {
		return Evidence{}, false
	}

	issues := collectGradleCompilerIssues(firstPart)
	reconcileGradleCompilerIssuesByErrorCount(firstPart, &issues)
	if len(issues) == 0 {
		return Evidence{}, false
	}

	groupBySig := map[string]*gradleErrorGroup{}
	order := make([]string, 0, len(issues))
	for _, issue := range issues {
		sigKey, errEntry, fileRef, ok := parseGradleKotlinIssueBlock(issue)
		if !ok {
			continue
		}
		current, exists := groupBySig[sigKey]
		if !exists {
			groupBySig[sigKey] = &gradleErrorGroup{entry: errEntry}
			order = append(order, sigKey)
			current = groupBySig[sigKey]
		}
		current.files = append(current.files, fileRef)
	}

	if len(groupBySig) == 0 {
		return Evidence{}, false
	}

	return Evidence{
		Task:   m[1],
		Errors: renderGroupedGradleErrors(order, groupBySig),
	}, true
}

func renderGroupedGradleErrors(order []string, groupBySig map[string]*gradleErrorGroup) []Error {
	errorsOut := make([]Error, 0, len(order))
	for _, sig := range order {
		group := groupBySig[sig]
		files := dedupeGradleFileRefs(group.files)
		entry := group.entry
		normalizeGradleFileRefs(&entry, files)
		errorsOut = append(errorsOut, entry)
	}
	return errorsOut
}

func dedupeGradleFileRefs(files []fileRef) []fileRef {
	seen := make(map[string]struct{}, len(files))
	out := make([]fileRef, 0, len(files))
	for _, f := range files {
		key := fmt.Sprintf("%s:%s", f.Path, f.Snippet)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out
}

func parseGradleKotlinIssueBlock(block []string) (string, Error, fileRef, bool) {
	if len(block) == 0 {
		return "", Error{}, fileRef{}, false
	}
	m := gradleKotlinErrorLineRe.FindStringSubmatch(strings.TrimSpace(block[0]))
	if len(m) != 5 {
		return "", Error{}, fileRef{}, false
	}
	message := strings.TrimSpace(m[4])
	snippet := ""
	for _, raw := range block[1:] {
		lineText := strings.TrimSpace(raw)
		if lineText == "" || lineText == "^" {
			continue
		}
		if snippet == "" {
			snippet = lineText
		}
	}

	filePath := strings.TrimPrefix(strings.TrimSpace(m[1]), "file://")
	pathWithLocation := filePath + ":" + strings.TrimSpace(m[2]) + ":" + strings.TrimSpace(m[3])
	return message, Error{
			Message: message,
		}, fileRef{
			Path:    pathWithLocation,
			Snippet: snippet,
		}, true
}

func parseGradleJavaIssueBlock(block []string) (string, Error, fileRef, bool) {
	if len(block) == 0 {
		return "", Error{}, fileRef{}, false
	}
	m := gradleJavaErrorLineRe.FindStringSubmatch(strings.TrimSpace(block[0]))
	if len(m) != 4 {
		return "", Error{}, fileRef{}, false
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
	sigKey := strings.Join(sigParts, " | ")

	pathWithLine := strings.TrimSpace(m[1]) + ":" + strings.TrimSpace(m[2])
	return sigKey, Error{
			Message:  message,
			Symbol:   symbol,
			Location: location,
		}, fileRef{
			Path:    pathWithLine,
			Snippet: snippet,
		}, true
}

func normalizeGradleFileRefs(entry *Error, files []fileRef) {
	if entry == nil || len(files) == 0 {
		return
	}
	if snippet, ok := commonSnippet(files); ok {
		entry.Snippet = snippet
		for i := range files {
			files[i].Snippet = ""
		}
	}
	if base, ok := commonBasePath(files); ok {
		entry.Base = base
		for i := range files {
			filePath, location, hasLocation := splitPathAndLocation(files[i].Path)
			trimmed := strings.TrimPrefix(filePath, base)
			if trimmed == "" || trimmed == filePath {
				continue
			}
			if hasLocation {
				files[i].Path = trimmed + location
				continue
			}
			files[i].Path = trimmed
		}
	}
	entry.Files = make([]string, 0, len(files))
	for _, file := range files {
		entry.Files = append(entry.Files, file.Path)
	}
}

func commonSnippet(files []fileRef) (string, bool) {
	if len(files) == 0 {
		return "", false
	}
	candidate := files[0].Snippet
	if strings.TrimSpace(candidate) == "" {
		return "", false
	}
	for i := 1; i < len(files); i++ {
		if files[i].Snippet != candidate {
			return "", false
		}
	}
	return candidate, true
}

func commonBasePath(files []fileRef) (string, bool) {
	if len(files) == 0 {
		return "", false
	}
	dir := ""
	for i := range files {
		filePath, _, _ := splitPathAndLocation(files[i].Path)
		slash := strings.LastIndex(filePath, "/")
		if slash <= 0 {
			return "", false
		}
		currentDir := filePath[:slash+1]
		if i == 0 {
			dir = currentDir
			continue
		}
		dir = sharedPrefix(dir, currentDir)
		if dir == "" {
			return "", false
		}
	}
	lastSlash := strings.LastIndex(dir, "/")
	if lastSlash < 0 {
		return "", false
	}
	base := dir[:lastSlash+1]
	if base == "" {
		return "", false
	}
	return base, true
}

func splitPathAndLocation(pathWithOptionalLocation string) (string, string, bool) {
	filePath, last, ok := splitPathTrailingInt(pathWithOptionalLocation)
	if !ok {
		return pathWithOptionalLocation, "", false
	}
	if path, line, hasLine := splitPathTrailingInt(filePath); hasLine {
		return path, ":" + strconv.Itoa(line) + ":" + strconv.Itoa(last), true
	}
	return filePath, ":" + strconv.Itoa(last), true
}

func splitPathTrailingInt(pathWithTrailingInt string) (string, int, bool) {
	idx := strings.LastIndex(pathWithTrailingInt, ":")
	if idx <= 0 || idx >= len(pathWithTrailingInt)-1 {
		return pathWithTrailingInt, 0, false
	}
	value, err := strconv.Atoi(pathWithTrailingInt[idx+1:])
	if err != nil {
		return pathWithTrailingInt, 0, false
	}
	return pathWithTrailingInt[:idx], value, true
}

func sharedPrefix(a, b string) string {
	if a == "" || b == "" {
		return ""
	}
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	i := 0
	for i < limit && a[i] == b[i] {
		i++
	}
	return a[:i]
}

func buildGradlePluginApplyPayload(secondPart []string) (Evidence, bool) {
	if len(secondPart) == 0 {
		return Evidence{}, false
	}
	joined := strings.Join(secondPart, "\n")
	if !strings.Contains(joined, "An exception occurred applying plugin request") {
		return Evidence{}, false
	}

	pluginID := ""
	pluginVersion := ""
	if match := gradlePluginRequestRe.FindStringSubmatch(joined); len(match) >= 2 {
		pluginID = strings.TrimSpace(match[1])
		if len(match) >= 3 {
			pluginVersion = strings.TrimSpace(match[2])
		}
	}

	block := strings.TrimSpace(extractGradlePluginApplyRootCause(secondPart))
	if block == "" {
		block = strings.TrimSpace(extractGradleWhatWentWrong(secondPart))
	}
	if block == "" {
		block = normalizeMultilineLimit(joined, 20)
	}
	if block == "" {
		return Evidence{}, false
	}

	return Evidence{
		Errors: []Error{{
			Message: block,
			Plugin:  pluginID,
			Version: pluginVersion,
		}},
	}, true
}

func extractGradlePluginApplyRootCause(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	start := -1
	for i, line := range lines {
		if strings.Contains(line, "Failed to apply plugin") {
			start = i + 1
			break
		}
	}
	if start == -1 || start >= len(lines) {
		return ""
	}

	collected := make([]string, 0, 4)
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "BUILD FAILED") {
			break
		}
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
		if trimmed == "" {
			continue
		}
		collected = append(collected, trimmed)
	}
	return strings.Join(collected, "\n")
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

func boundMessage(text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > maxMessageLines {
		lines = lines[:maxMessageLines]
		text = strings.Join(lines, "\n")
		if strings.HasSuffix(text, "\n") {
			text = strings.TrimRight(text, "\n")
		}
	}
	if len(text) <= maxMessageBytes {
		return text
	}
	return string([]byte(text)[:maxMessageBytes])
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
	for end < len(lines) && isGradleTryBlockLine(strings.TrimSpace(lines[end])) {
		end++
	}
	return append(slicesClone(lines[:start]), lines[end:]...)
}

func removeGradleInterleavedNoiseLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if isGradleInterleavedNoiseLine(strings.TrimSpace(line)) {
			continue
		}
		out = append(out, line)
	}
	return out
}

func removeGradleExceptionBlock(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) != "* Exception is:" {
			out = append(out, lines[i])
			i++
			continue
		}
		i++
		for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "BUILD FAILED") {
			i++
		}
	}
	return out
}

func isGradleTryBlockLine(line string) bool {
	return line == "" || strings.HasPrefix(line, "> ") || isGradleInterleavedNoiseLine(line)
}

func isGradleInterleavedNoiseLine(line string) bool {
	return strings.HasPrefix(line, "Cached resource ") ||
		strings.HasPrefix(line, "Failed to get resource: HEAD.") ||
		strings.HasPrefix(line, "Build cache key for task ") ||
		strings.HasPrefix(line, "Skipping task ") ||
		strings.HasPrefix(line, "gradle/actions: Writing build results")
}

func removeGradleDeprecationFooter(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		if !isGradleDeprecationFooterStart(strings.TrimSpace(lines[i])) {
			out = append(out, lines[i])
			i++
			continue
		}
		i++
		for i < len(lines) {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" || isGradleDeprecationFooterLine(trimmed) {
				i++
				continue
			}
			break
		}
	}
	return out
}

func trimGradleAfterTerminalFailure(lines []string) []string {
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "BUILD FAILED") {
			continue
		}
		out := slicesClone(lines[:i+1])
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" {
				continue
			}
			if gradleActionableTasksRe.MatchString(trimmed) {
				out = append(out, lines[j])
			}
			break
		}
		return out
	}
	return lines
}

func isGradleDeprecationFooterStart(line string) bool {
	return strings.HasPrefix(line, "Deprecated Gradle features were used in this build")
}

func isGradleDeprecationFooterLine(line string) bool {
	return isGradleDeprecationFooterStart(line) ||
		strings.HasPrefix(line, "You can use '--warning-mode all'") ||
		strings.HasPrefix(line, "For more on this, please refer to https://docs.gradle.org/")
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

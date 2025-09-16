package mods

import (
	"regexp"
	"strconv"
	"strings"
)

// ParsedBuildError represents a normalized compiler/build error location
type ParsedBuildError struct {
	Language string
	Tool     string
	File     string
	Line     int
	Column   int
	Message  string
}

// BuildErrorParser parses raw build output into normalized errors
type BuildErrorParser interface {
	Parse(raw string) []ParsedBuildError
}

// JavaMavenErrorParser extracts errors from Maven compiler output
// Supports forms like:
//
//	[ERROR] /path/File.java:[4,9] cannot find symbol
//	[ERROR] /path/File.java:4: cannot find symbol (less common)
type JavaMavenErrorParser struct{}

func (JavaMavenErrorParser) Parse(raw string) []ParsedBuildError {
	var out []ParsedBuildError
	// Patterns: capture file, line, optional column, and trailing message
	// 1) File.java:[line,col] message
	reBracket := regexp.MustCompile(`(?m)([A-Za-z0-9_./\\-]+\.java):\[(\d+),(\d+)\]\s+(.+)$`)
	// 2) File.java:line(:col)? message
	reColon := regexp.MustCompile(`(?m)([A-Za-z0-9_./\\-]+\.java):(\d+)(?::(\d+))?\s+(.+)$`)

	matches := reBracket.FindAllStringSubmatch(raw, -1)
	for _, m := range matches {
		if len(m) < 5 {
			continue
		}
		ln, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		out = append(out, ParsedBuildError{
			Language: "java",
			Tool:     "maven",
			File:     normalizePath(m[1]),
			Line:     ln,
			Column:   col,
			Message:  strings.TrimSpace(m[4]),
		})
	}
	matches = reColon.FindAllStringSubmatch(raw, -1)
	for _, m := range matches {
		if len(m) < 5 {
			continue
		}
		ln, _ := strconv.Atoi(m[2])
		col := 0
		if m[3] != "" {
			col, _ = strconv.Atoi(m[3])
		}
		out = append(out, ParsedBuildError{
			Language: "java",
			Tool:     "maven",
			File:     normalizePath(m[1]),
			Line:     ln,
			Column:   col,
			Message:  strings.TrimSpace(m[4]),
		})
	}
	return out
}

func normalizePath(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// ParseBuildErrors dispatches to a language/tool specific parser. For now, only Java/Maven.
func ParseBuildErrors(language, tool, raw string) []ParsedBuildError {
	language = strings.ToLower(strings.TrimSpace(language))
	// tool parameter currently unused for dispatch; mark as used for future extension
	_ = tool
	switch language {
	case "java":
		// Assume Maven for now; Gradle compiler output is often similar and may also match
		return JavaMavenErrorParser{}.Parse(raw)
	default:
		return nil
	}
}

package build

import (
	"regexp"
	"strconv"
	"strings"
)

// ParsedBuildError represents a normalized compiler/build error location.
type ParsedBuildError struct {
	Language string
	Tool     string
	File     string
	Line     int
	Column   int
	Message  string
}

// ParseBuildErrors dispatches to a language/tool specific parser.
func ParseBuildErrors(language, tool, raw string) []ParsedBuildError {
	language = strings.ToLower(strings.TrimSpace(language))
	tool = strings.ToLower(strings.TrimSpace(tool))
	switch language {
	case "java":
		return parseJavaErrors(tool, raw)
	default:
		return nil
	}
}

func parseJavaErrors(tool, raw string) []ParsedBuildError {
	var out []ParsedBuildError
	reBracket := regexp.MustCompile(`(?m)([A-Za-z0-9_./\\-]+\.java):\[(\d+),(\d+)\]\s+(.+)$`)
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
			Tool:     defaultString(tool, "maven"),
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
			Tool:     defaultString(tool, "maven"),
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

func defaultString(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

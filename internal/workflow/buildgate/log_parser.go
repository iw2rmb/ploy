package buildgate

import (
	"bufio"
	"strings"
)

// LogFinding represents a normalized issue extracted from build logs.
type LogFinding struct {
	Code     string
	Severity string
	Message  string
	Evidence string
}

// LogParser converts raw build logs into knowledge base findings.
type LogParser interface {
	Parse(log string) []LogFinding
}

type logPattern struct {
	code     string
	severity string
	message  string
	match    func(string) bool
}

// DefaultLogParser extracts common compiler and infrastructure failures from build logs.
type DefaultLogParser struct {
	patterns []logPattern
}

// NewDefaultLogParser constructs the default build log parser configured with known patterns.
func NewDefaultLogParser() *DefaultLogParser {
	return &DefaultLogParser{
		patterns: []logPattern{
			{
				code:     "kb.git.auth",
				severity: "error",
				message:  "Authenticate Git fetch credentials for remote repository access.",
				match: func(line string) bool {
					return strings.Contains(line, "fatal error: unable to access") ||
						(strings.Contains(line, "permission denied") && strings.Contains(line, "git")) ||
						strings.Contains(line, "authentication failed")
				},
			},
			{
				code:     "kb.go.module_conflict",
				severity: "error",
				message:  "Resolve conflicting Go modules or tidy dependencies to eliminate duplicate definitions.",
				match: func(line string) bool {
					return strings.Contains(line, "go: module") && strings.Contains(line, "found in multiple modules")
				},
			},
			{
				code:     "kb.linker.undefined_reference",
				severity: "error",
				message:  "Linker could not resolve a symbol; ensure dependent libraries are included.",
				match: func(line string) bool {
					return strings.Contains(line, "undefined reference to")
				},
			},
			{
				code:     "kb.infrastructure.disk_full",
				severity: "error",
				message:  "Build host ran out of disk space; clear caches or provision additional storage.",
				match: func(line string) bool {
					return strings.Contains(line, "no space left on device") || strings.Contains(line, "disk quota exceeded")
				},
			},
		},
	}
}

// Parse analyses the log content and returns normalized findings.
func (p *DefaultLogParser) Parse(log string) []LogFinding {
	if p == nil {
		return nil
	}
	scanner := bufio.NewScanner(strings.NewReader(log))
	seen := make(map[string]struct{})
	var findings []LogFinding
	for scanner.Scan() {
		rawLine := strings.TrimSpace(scanner.Text())
		if rawLine == "" {
			continue
		}
		lower := strings.ToLower(rawLine)
		for _, pattern := range p.patterns {
			if pattern.match(lower) {
				key := pattern.code + "::" + rawLine
				if _, ok := seen[key]; ok {
					continue
				}
				findings = append(findings, LogFinding{
					Code:     pattern.code,
					Severity: pattern.severity,
					Message:  pattern.message,
					Evidence: rawLine,
				})
				seen[key] = struct{}{}
				break
			}
		}
	}
	return findings
}

// Ensure DefaultLogParser satisfies the LogParser interface.
var _ LogParser = (*DefaultLogParser)(nil)

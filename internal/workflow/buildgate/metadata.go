package buildgate

import "strings"

// Metadata captures build gate execution metadata emitted by workflow stages.
type Metadata struct {
	LogDigest    string
	StaticChecks []StaticCheckReport
}

// StaticCheckReport summarises a static analysis tool invocation.
type StaticCheckReport struct {
	Language string
	Tool     string
	Passed   bool
	Failures []StaticCheckFailure
}

// StaticCheckFailure records a single diagnostic reported by a static check tool.
type StaticCheckFailure struct {
	RuleID   string
	File     string
	Line     int
	Column   int
	Severity string
	Message  string
}

// Sanitize trims string fields, clamps numeric values, and filters empty entries.
func Sanitize(meta Metadata) Metadata {
	result := Metadata{LogDigest: strings.TrimSpace(meta.LogDigest)}
	for _, check := range meta.StaticChecks {
		sanitized := StaticCheckReport{
			Language: strings.TrimSpace(check.Language),
			Tool:     strings.TrimSpace(check.Tool),
			Passed:   check.Passed,
		}
		for _, failure := range check.Failures {
			ruleID := strings.TrimSpace(failure.RuleID)
			file := strings.TrimSpace(failure.File)
			message := strings.TrimSpace(failure.Message)
			severity := normalizeSeverity(strings.TrimSpace(failure.Severity))
			line := failure.Line
			if line < 0 {
				line = 0
			}
			column := failure.Column
			if column < 0 {
				column = 0
			}
			if message == "" && ruleID == "" && file == "" {
				continue
			}
			sanitized.Failures = append(sanitized.Failures, StaticCheckFailure{
				RuleID:   ruleID,
				File:     file,
				Line:     line,
				Column:   column,
				Severity: severity,
				Message:  message,
			})
		}
		if len(sanitized.Failures) == 0 && sanitized.Passed && sanitized.Tool == "" && sanitized.Language == "" {
			continue
		}
		result.StaticChecks = append(result.StaticChecks, sanitized)
	}
	return result
}

func normalizeSeverity(value string) string {
	switch strings.ToLower(value) {
	case "warning":
		return "warning"
	case "info":
		return "info"
	case "error":
		return "error"
	default:
		return "error"
	}
}

package buildgate

import (
	"errors"
	"fmt"
	"strings"
)

func sanitizeAdapterMetadata(meta StaticCheckAdapterMetadata) (StaticCheckAdapterMetadata, error) {
	language := normalizeLanguage(meta.Language)
	if language == "" {
		return StaticCheckAdapterMetadata{}, errors.New("language is required")
	}
	tool := strings.TrimSpace(meta.Tool)
	if tool == "" {
		return StaticCheckAdapterMetadata{}, errors.New("tool is required")
	}
	severity := meta.DefaultSeverity
	if severity == "" {
		severity = SeverityError
	}
	normalizedSeverity, err := normalizeSeverityLevel(severity)
	if err != nil {
		return StaticCheckAdapterMetadata{}, err
	}
	return StaticCheckAdapterMetadata{
		Language:        language,
		Tool:            tool,
		DefaultSeverity: normalizedSeverity,
	}, nil
}

func normalizeSeverityLevel(level SeverityLevel) (SeverityLevel, error) {
	switch level {
	case "":
		return "", nil
	case SeverityError, SeverityWarning, SeverityInfo:
		return level, nil
	default:
		return "", fmt.Errorf("invalid severity level %q", string(level))
	}
}

func parseSeverityLevel(value string) (SeverityLevel, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", errors.New("severity is required")
	}
	switch strings.ToLower(trimmed) {
	case string(SeverityError):
		return SeverityError, nil
	case string(SeverityWarning):
		return SeverityWarning, nil
	case string(SeverityInfo):
		return SeverityInfo, nil
	default:
		return "", fmt.Errorf("invalid severity level %q", trimmed)
	}
}

func normalizeLanguage(language string) string {
	normalized := strings.ToLower(strings.TrimSpace(language))
	switch normalized {
	case "":
		return ""
	case "go", "golang":
		return "golang"
	case "js", "node", "nodejs", "javascript":
		return "javascript"
	case "ts", "typescript":
		return "typescript"
	case "py", "python":
		return "python"
	case "c#", "csharp":
		return "csharp"
	default:
		return normalized
	}
}

func copyOptions(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[strings.TrimSpace(k)] = v
	}
	return dst
}

func clampNonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func severityGreaterOrEqual(severity SeverityLevel, threshold SeverityLevel) bool {
	return severityRank(severity) >= severityRank(threshold)
}

func severityRank(level SeverityLevel) int {
	switch level {
	case SeverityInfo:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

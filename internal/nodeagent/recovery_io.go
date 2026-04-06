// recovery_io.go contains shared recovery parsers/helpers used by discrete gate/heal jobs.
package nodeagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	orwStatsMetadataErrorKind = "orw_error_kind"
	orwStatsMetadataReason    = "orw_reason"
)

// parseBugSummary reads /out/heal.json and extracts the "bug_summary" field
// from a JSON one-liner. Returns an empty string if the file is missing, unreadable,
// or does not contain a bug_summary field.
func parseBugSummary(outDir string) string {
	return parseCodexLastField(outDir, "bug_summary")
}

// parseActionSummary reads /out/heal.json and extracts the "action_summary"
// field from a JSON one-liner. Returns an empty string if the file is missing,
// unreadable, or does not contain an action_summary field.
func parseActionSummary(outDir string) string {
	return parseCodexLastField(outDir, "action_summary")
}

// parseErrorKind reads /out/heal.json and extracts the "error_kind" field.
// Returns empty string when the file/field is unavailable.
func parseErrorKind(outDir string) string {
	return parseCodexLastField(outDir, "error_kind")
}

// parseORWFailureMetadata reads /out/report.json and extracts deterministic ORW
// failure fields for run stats metadata. Missing report.json returns (nil, nil).
func parseORWFailureMetadata(outDir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(outDir, contracts.ORWCLIReportFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", contracts.ORWCLIReportFileName, err)
	}

	report, err := contracts.ParseORWCLIReport(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", contracts.ORWCLIReportFileName, err)
	}
	if report.Success {
		return nil, nil
	}

	meta := map[string]string{
		orwStatsMetadataErrorKind: string(report.ErrorKind),
	}
	if strings.TrimSpace(report.Reason) != "" {
		meta[orwStatsMetadataReason] = strings.TrimSpace(report.Reason)
	}
	return meta, nil
}

// parseCodexLastField reads heal.json from outDir and extracts a named string
// field from the JSON content. The file is expected to contain one or more lines;
// each line is tried as a JSON object. The first line containing the requested field
// wins. The returned value is trimmed and truncated to 200 characters.
func parseCodexLastField(outDir, field string) string {
	data, err := os.ReadFile(filepath.Join(outDir, "heal.json"))
	if err != nil {
		return ""
	}

	// Try each line as a potential JSON object.
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		if val, ok := obj[field]; ok {
			if s, ok := val.(string); ok {
				return truncateOneLine(s, 200)
			}
		}
	}

	// If line-by-line didn't work, try the entire content as a single JSON object
	// (in case the file is a single-line JSON without trailing newline).
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err == nil {
		if val, ok := obj[field]; ok {
			if s, ok := val.(string); ok {
				return truncateOneLine(s, 200)
			}
		}
	}

	return ""
}

func truncateOneLine(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	if maxRunes == 1 {
		return "…"
	}
	r := []rune(s)
	return string(r[:maxRunes-1]) + "…"
}

func gateLogPayloadFromMetadata(gateMetadata *contracts.BuildGateStageMetadata) string {
	if gateMetadata == nil {
		return ""
	}
	logPayload := gateMetadata.LogsText
	if len(gateMetadata.LogFindings) > 0 {
		if trimmed := strings.TrimSpace(gateMetadata.LogFindings[0].Message); trimmed != "" {
			logPayload = trimmed
			if !strings.HasSuffix(logPayload, "\n") {
				logPayload += "\n"
			}
		}
	}
	return logPayload
}

// recovery_io.go contains shared recovery parsers/helpers used by discrete gate/heal jobs.
package nodeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// parseBugSummary reads /out/codex-last.txt and extracts the "bug_summary" field
// from a JSON one-liner. Returns an empty string if the file is missing, unreadable,
// or does not contain a bug_summary field.
func parseBugSummary(outDir string) string {
	return parseCodexLastField(outDir, "bug_summary")
}

// parseActionSummary reads /out/codex-last.txt and extracts the "action_summary"
// field from a JSON one-liner. Returns an empty string if the file is missing,
// unreadable, or does not contain an action_summary field.
func parseActionSummary(outDir string) string {
	return parseCodexLastField(outDir, "action_summary")
}

// parseRouterDecision reads /out/codex-last.txt and extracts structured
// classifier fields for recovery metadata. It always returns non-nil metadata
// with deterministic fallback values for loop/error kinds.
func parseRouterDecision(outDir string) *contracts.BuildGateRecoveryMetadata {
	decision := &contracts.BuildGateRecoveryMetadata{
		LoopKind:  "healing",
		ErrorKind: "unknown",
	}
	obj, ok := parseCodexLastJSONObject(outDir)
	if !ok {
		return decision
	}

	if val, ok := obj["error_kind"].(string); ok {
		switch strings.TrimSpace(val) {
		case "infra", "code", "mixed", "unknown", "custom":
			decision.ErrorKind = strings.TrimSpace(val)
		}
	}
	if val, ok := obj["strategy_id"].(string); ok {
		decision.StrategyID = truncateOneLine(val, 200)
	}
	if val, ok := obj["reason"].(string); ok {
		decision.Reason = truncateOneLine(val, 200)
	}
	if val, ok := obj["confidence"].(float64); ok {
		decision.Confidence = &val
	}
	if raw, ok := obj["expectations"]; ok {
		switch raw.(type) {
		case map[string]any, []any:
			if b, err := json.Marshal(raw); err == nil {
				decision.Expectations = b
			}
		}
	}
	if err := decision.Validate(); err != nil {
		decision.ErrorKind = "unknown"
		decision.StrategyID = ""
		decision.Confidence = nil
		decision.Reason = ""
		decision.Expectations = nil
	}
	return decision
}

// parseCodexLastField reads codex-last.txt from outDir and extracts a named string
// field from the JSON content. The file is expected to contain one or more lines;
// each line is tried as a JSON object. The first line containing the requested field
// wins. The returned value is trimmed and truncated to 200 characters.
func parseCodexLastField(outDir, field string) string {
	data, err := os.ReadFile(filepath.Join(outDir, "codex-last.txt"))
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

func parseCodexLastJSONObject(outDir string) (map[string]any, bool) {
	data, err := os.ReadFile(filepath.Join(outDir, "codex-last.txt"))
	if err != nil {
		return nil, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err == nil {
			return obj, true
		}
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err == nil {
		return obj, true
	}
	return nil, false
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

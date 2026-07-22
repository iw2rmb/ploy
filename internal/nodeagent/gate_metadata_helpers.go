package nodeagent

import (
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const nonExecutableGradlewErrorPrefix = "Non-executable gradlew:"

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

func gateFailureErrorFromMetadata(gateMetadata *contracts.BuildGateStageMetadata) string {
	if gateMetadata == nil {
		return ""
	}

	for _, finding := range gateMetadata.LogFindings {
		if line := errorLineContaining(finding.Message, nonExecutableGradlewErrorPrefix); line != "" {
			return boundedErrorText(line)
		}
	}
	if line := errorLineContaining(gateMetadata.LogsText, nonExecutableGradlewErrorPrefix); line != "" {
		return boundedErrorText(line)
	}

	for _, finding := range gateMetadata.LogFindings {
		if summary := normalizedErrorText(finding.Message); summary != "" {
			return boundedErrorText(summary)
		}
	}
	for _, line := range strings.Split(gateMetadata.LogsText, "\n") {
		if summary := normalizedErrorText(line); summary != "" {
			return boundedErrorText(summary)
		}
	}
	return ""
}

func errorLineContaining(text, marker string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, marker) {
			return normalizedErrorText(line)
		}
	}
	return ""
}

func normalizedErrorText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

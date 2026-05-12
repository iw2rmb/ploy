package step

import (
	"crypto/sha256"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// buildGateExecutionMetadata normalizes a finished gate container run into
// BuildGateStageMetadata. On success it surfaces gradle build-cache hits; on
// failure it extracts a tool-aware log finding from the captured output.
func buildGateExecutionMetadata(
	workspace string,
	language string,
	tool string,
	release string,
	image string,
	executedCommand string,
	res ContainerResult,
	logs []byte,
) *contracts.BuildGateStageMetadata {
	passed := res.ExitCode == 0
	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{
			Language: language,
			Tool:     tool,
			Passed:   passed,
		}},
		Detected: &contracts.StackExpectation{
			Language: strings.TrimSpace(language),
			Tool:     strings.TrimSpace(tool),
			Release:  strings.TrimSpace(release),
		},
		RuntimeImage:    image,
		ExecutedCommand: strings.TrimSpace(executedCommand),
	}
	if passed && strings.EqualFold(tool, "gradle") {
		if hits := readGradleBuildCacheHits(workspace); len(hits) > 0 {
			meta.LogFindings = append(meta.LogFindings, contracts.BuildGateLogFinding{
				Severity: "info",
				Code:     "GRADLE_BUILD_CACHE_HIT",
				Message:  fmt.Sprintf("gradle build cache hits (%d): %s", len(hits), strings.Join(hits, ", ")),
			})
		}
	}
	if !passed {
		trimmed, evidence := BuildGateLogFindingContent(tool, string(logs))
		msg := strings.TrimSpace(trimmed)
		if msg == "" {
			msg = fmt.Sprintf("%s build failed (exit %d)", tool, res.ExitCode)
		}
		finding := contracts.BuildGateLogFinding{Severity: "error", Message: msg}
		if strings.TrimSpace(evidence) != "" {
			finding.Evidence = evidence
		}
		meta.LogFindings = append(meta.LogFindings, finding)
	}
	attachLogsTextAndDigest(meta, logs)
	return meta
}

// gateProfileCommandFromContainerCommand returns the user-facing tool command
// for persistence in the gate profile, stripping our internal /bin/sh -lc and
// CA preamble wrapper when present.
func gateProfileCommandFromContainerCommand(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	if len(cmd) >= 3 && cmd[0] == "/bin/sh" && (cmd[1] == "-c" || cmd[1] == "-lc") {
		shell := strings.TrimSpace(cmd[2])
		if shell == "" {
			return ""
		}
		prefixes := []string{
			"set -eu; " + gateCAPreamble + "; ",
			gateCAPreamble + "; ",
		}
		for _, prefix := range prefixes {
			if rest, ok := strings.CutPrefix(shell, prefix); ok {
				shell = strings.TrimSpace(rest)
				break
			}
		}
		return shell
	}
	return strings.TrimSpace(strings.Join(cmd, " "))
}

func attachLogsTextAndDigest(meta *contracts.BuildGateStageMetadata, logs []byte) {
	const maxLogBytes = 10 << 20 // 10 MiB safety cap in memory
	if len(logs) > maxLogBytes {
		logs = logs[:maxLogBytes]
	}
	meta.LogsText = string(logs)
	meta.LogDigest = sha256Digest(logs)
}

func sha256Digest(b []byte) types.Sha256Digest {
	h := sha256.Sum256(b)
	return types.Sha256Digest(fmt.Sprintf("sha256:%x", h[:]))
}

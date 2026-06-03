package step

import (
	"crypto/sha256"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// gateExecutionMetadata normalizes a finished gate container run into
// BuildGateStageMetadata. On success it surfaces gradle build-cache hits; on
// failure it preserves the captured output without tool-specific trimming.
func gateExecutionMetadata(
	workspace string,
	language string,
	tool string,
	release string,
	image string,
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
		RuntimeImage: image,
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
		msg := string(capGateLogBytes(logs))
		if msg == "" {
			msg = fmt.Sprintf("%s build failed (exit %d)", tool, res.ExitCode)
		}
		meta.LogFindings = append(meta.LogFindings, contracts.BuildGateLogFinding{Severity: "error", Message: msg})
	}
	attachLogsTextAndDigest(meta, logs)
	return meta
}

func attachLogsTextAndDigest(meta *contracts.BuildGateStageMetadata, logs []byte) {
	logs = capGateLogBytes(logs)
	meta.LogsText = string(logs)
	meta.LogDigest = sha256Digest(logs)
}

func capGateLogBytes(logs []byte) []byte {
	const maxLogBytes = 10 << 20 // 10 MiB safety cap in memory
	if len(logs) > maxLogBytes {
		return logs[:maxLogBytes]
	}
	return logs
}

func sha256Digest(b []byte) types.Sha256Digest {
	h := sha256.Sum256(b)
	return types.Sha256Digest(fmt.Sprintf("sha256:%x", h[:]))
}

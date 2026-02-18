package nodeagent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func ensureHealingInDir(inDir *string, runID types.RunID) error {
	if *inDir != "" {
		return nil
	}
	tmpInDir, dirErr := os.MkdirTemp("", "ploy-mod-in-*")
	if dirErr != nil {
		slog.Error("failed to create /in directory for healing", "run_id", runID, "error", dirErr)
		return dirErr
	}
	*inDir = tmpInDir
	return nil
}

func gateLogPayloadFromMetadata(gateMetadata *contracts.BuildGateStageMetadata) string {
	if gateMetadata == nil {
		return ""
	}
	// Prefer trimmed log view (LogFindings) when available so Codex and
	// other healing mods see a focused failure slice instead of the full truncated gate log.
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

func persistBuildGateLog(inDir, logPayload string, runID types.RunID, gatePhase string) {
	if logPayload == "" || inDir == "" {
		return
	}
	inLogPath := filepath.Join(inDir, "build-gate.log")
	if writeErr := os.WriteFile(inLogPath, []byte(logPayload), 0o644); writeErr != nil {
		slog.Warn("failed to write /in/build-gate.log", "run_id", runID, "error", writeErr)
	} else {
		slog.Info("build-gate.log persisted to /in for healing", "run_id", runID, "path", inLogPath, "phase", gatePhase)
	}
}

func persistBuildGateIterationLog(inDir, logPayload string, attempt int, runID types.RunID) {
	if logPayload == "" || inDir == "" {
		return
	}
	iterGateLogPath := filepath.Join(inDir, fmt.Sprintf("build-gate-iteration-%d.log", attempt))
	if writeErr := os.WriteFile(iterGateLogPath, []byte(logPayload), 0o644); writeErr != nil {
		slog.Warn("failed to write build-gate-iteration log", "run_id", runID, "attempt", attempt, "error", writeErr)
	}
}

func persistHealingIterationLog(inDir, outDir string, attempt int, runID types.RunID) {
	if inDir == "" {
		return
	}
	healingIterLogPath := filepath.Join(inDir, fmt.Sprintf("healing-iteration-%d.log", attempt))
	if codexLog, readErr := os.ReadFile(filepath.Join(outDir, "codex.log")); readErr == nil {
		if writeErr := os.WriteFile(healingIterLogPath, codexLog, 0o644); writeErr != nil {
			slog.Warn("failed to write healing-iteration log", "run_id", runID, "attempt", attempt, "error", writeErr)
		}
	}
}

func appendHealingLogEntry(buf *strings.Builder, attempt int, bugSummary, actionSummary string) {
	if attempt == 1 {
		buf.WriteString("# Healing Log\n\n")
	}
	fmt.Fprintf(buf, "## Iteration %d\n\n", attempt)
	if bugSummary != "" {
		fmt.Fprintf(buf, "- Bug Summary: %s\n", bugSummary)
	} else {
		buf.WriteString("- Bug Summary: N/A\n")
	}
	fmt.Fprintf(buf, "  Build Log: /in/build-gate-iteration-%d.log\n", attempt)
	if actionSummary != "" {
		fmt.Fprintf(buf, "- Healing Attempt: %s\n", actionSummary)
	} else {
		buf.WriteString("- Healing Attempt: N/A\n")
	}
	fmt.Fprintf(buf, "  Agent Log: /in/healing-iteration-%d.log\n\n", attempt)
}

func persistHealingLog(inDir string, buf *strings.Builder, runID types.RunID) {
	if inDir == "" {
		return
	}
	healingLogPath := filepath.Join(inDir, "healing-log.md")
	if writeErr := os.WriteFile(healingLogPath, []byte(buf.String()), 0o644); writeErr != nil {
		slog.Warn("failed to write healing-log.md", "run_id", runID, "error", writeErr)
	}
}

func persistUpdatedBuildGateLog(inDir, logPayload string, runID types.RunID) {
	if logPayload == "" || inDir == "" {
		return
	}
	inLogPath := filepath.Join(inDir, "build-gate.log")
	if writeErr := os.WriteFile(inLogPath, []byte(logPayload), 0o644); writeErr != nil {
		slog.Warn("failed to update /in/build-gate.log after re-gate", "run_id", runID, "error", writeErr)
	}
}

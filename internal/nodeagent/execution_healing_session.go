package nodeagent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func readHealingSessionFromOutDir(outDir string, runID types.RunID, healingIndex int) string {
	sessionBytes, readErr := os.ReadFile(filepath.Join(outDir, "codex-session.txt"))
	if readErr != nil {
		return ""
	}
	session := strings.TrimSpace(string(sessionBytes))
	if session == "" {
		return ""
	}
	slog.Info("healing: captured session from /out", "run_id", runID, "healing_index", healingIndex, "session_id", session)
	return session
}

func persistHealingSessionToInDir(inDir, session string, runID types.RunID) {
	if session == "" || inDir == "" {
		return
	}
	sessionPath := filepath.Join(inDir, "codex-session.txt")
	if writeErr := os.WriteFile(sessionPath, []byte(session), 0o644); writeErr != nil {
		slog.Warn("healing: failed to persist codex-session.txt into /in", "run_id", runID, "error", writeErr)
	} else {
		slog.Info("healing: persisted codex-session.txt to /in for resume", "run_id", runID, "session_id", session)
	}
}

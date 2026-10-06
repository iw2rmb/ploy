package nodeagent

import (
	"fmt"
	"math"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func (c *ClaimManager) uploadRecoveredTerminalStatus(jobID types.JobID, containerID string, terminal recoveredContainerTerminal, resumeCount ...int) error {
	exitCode, err := safeExitCodeInt32(terminal.ExitCode)
	if err != nil {
		return fmt.Errorf("normalize exit code: %w", err)
	}

	durationMs := int64(0)
	if !terminal.StartedAt.IsZero() && !terminal.FinishedAt.IsZero() && terminal.FinishedAt.After(terminal.StartedAt) {
		durationMs = terminal.FinishedAt.Sub(terminal.StartedAt).Milliseconds()
	}
	count := 0
	if len(resumeCount) > 0 {
		count = resumeCount[0]
	}
	stats := types.NewRunStatsBuilder().
		ResumeCount(count).
		ExitCode(int(exitCode)).
		DurationMs(durationMs).
		MetadataEntry("source", "startup_reconcile").
		MetadataEntry("container_id", containerID).
		MustBuild()
	status := lifecycle.JobStatusFromExitCode(int(exitCode))

	if err := c.uploadRecoveredJobStatus(jobID, status, &exitCode, stats); err != nil {
		return fmt.Errorf("upload status: %w", err)
	}
	return nil
}

func safeExitCodeInt32(exitCode int) (int32, error) {
	if exitCode < math.MinInt32 || exitCode > math.MaxInt32 {
		return 0, fmt.Errorf("exit code %d overflows int32", exitCode)
	}
	return int32(exitCode), nil
}

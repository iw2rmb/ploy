package app

import (
	"io"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/pull"
	"github.com/spf13/cobra"
)

// newPullCmd creates the cobra command for `ploy pull`.
func newPullCmd(stderr io.Writer) *cobra.Command {
	var origin string
	var newRun, follow, dryRun, cancelOnCap bool
	var capDuration time.Duration
	var maxRetries int
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull Migs diffs for current repo HEAD",
		Long: `Ensures a Migs run exists for the current local repo HEAD and pulls diffs.

Maintains per-repo pull state that binds {repo_url, head_sha, run_id}.

Behavior:
  - If no saved pull state: initiates a run (requires --follow or re-invocation)
  - If HEAD SHA mismatch: requires --new-run to initiate a fresh run
  - If SHA matches and run succeeded: pulls diffs
  - If --follow is set: follows run until terminal and then pulls diffs`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := []string{}
			runArgs = addChangedBool(cmd, runArgs, "new-run", newRun)
			runArgs = addChangedBool(cmd, runArgs, "follow", follow)
			runArgs = addChangedString(cmd, runArgs, "origin", origin)
			runArgs = addChangedBool(cmd, runArgs, "dry-run", dryRun)
			runArgs = addChangedDuration(cmd, runArgs, "cap", capDuration)
			runArgs = addChangedBool(cmd, runArgs, "cancel-on-cap", cancelOnCap)
			runArgs = addChangedInt(cmd, runArgs, "max-retries", maxRetries)
			return pull.Handle(runArgs, stderr)
		},
	}
	cmd.Flags().BoolVar(&newRun, "new-run", false, "Force initiating a new run")
	cmd.Flags().BoolVar(&follow, "follow", false, "Follow run until completion")
	cmd.Flags().StringVar(&origin, "origin", "origin", "Git remote to match")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and print actions without mutating")
	cmd.Flags().DurationVar(&capDuration, "cap", 0, "Optional time cap for --follow")
	cmd.Flags().BoolVar(&cancelOnCap, "cancel-on-cap", false, "Cancel run if cap exceeded")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 5, "Max SSE reconnect attempts")

	return cmd
}

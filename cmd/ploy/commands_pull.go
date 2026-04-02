package main

import (
	"io"

	"github.com/spf13/cobra"
)

// newPullCmd creates the cobra command for `ploy pull`.
func newPullCmd(stderr io.Writer) *cobra.Command {
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
		RunE: func(cmd *cobra.Command, args []string) error {
			// Collect all flags and positional args for handlePull.
			// Since we're using flag.FlagSet internally, pass the original args.
			return handlePull(args, stderr)
		},
		// Disable cobra's built-in flag parsing so handlePull can use flag.FlagSet.
		DisableFlagParsing: true,
	}

	return cmd
}

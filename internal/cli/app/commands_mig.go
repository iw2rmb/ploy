package app

import (
	"context"
	"io"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/mig"
	"github.com/iw2rmb/ploy/internal/cli/pull"
	"github.com/spf13/cobra"
)

// newMigCmd creates the cobra command tree for 'ploy mig' and its subcommands.
// This wires existing mig handlers into a proper cobra command hierarchy.
func newMigCmd(stderr io.Writer) *cobra.Command {
	migCmd := &cobra.Command{
		Use:   "mig",
		Short: "Plan and run Migs workflows",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	migCmd.AddCommand(newMigAddCmd(stderr))
	migCmd.AddCommand(&cobra.Command{Use: "list", Short: "List mig projects", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunList(context.Background(), stderr)
	}})
	migCmd.AddCommand(&cobra.Command{Use: "remove <mig-id|name>", Short: "Delete a mig project", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRemove(context.Background(), args[0], stderr)
	}})
	migCmd.AddCommand(&cobra.Command{Use: "archive <mig-id|name>", Short: "Archive a mig project", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunArchive(context.Background(), args[0], stderr)
	}})
	migCmd.AddCommand(&cobra.Command{Use: "unarchive <mig-id|name>", Short: "Unarchive a mig project", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunUnarchive(context.Background(), args[0], stderr)
	}})
	migCmd.AddCommand(newMigSpecCmd(stderr))
	migCmd.AddCommand(newMigRepoCmd(stderr))
	migCmd.AddCommand(newMigPullCmd(stderr))
	migCmd.AddCommand(&cobra.Command{Use: "status <mig-id>", Short: "Show migration status and per-run summary", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunStatus(context.Background(), args[0], stderr)
	}})
	migCmd.AddCommand(newMigRunCmd(stderr))
	migCmd.AddCommand(newMigFetchCmd(stderr))
	migCmd.AddCommand(&cobra.Command{Use: "artifacts <run-id>", Short: "List run artifacts by stage", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunArtifacts(context.Background(), args[0], stderr)
	}})

	return migCmd
}

func newMigAddCmd(stderr io.Writer) *cobra.Command {
	var name, spec string
	cmd := &cobra.Command{
		Use:   "add --name <name>",
		Short: "Create a new mig project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mig.RunAdd(context.Background(), mig.AddOptions{
				Name:     name,
				SpecPath: spec,
				Output:   stderr,
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Unique name for the mig")
	cmd.Flags().StringVar(&spec, "spec", "", "Path to YAML/JSON spec file")
	return cmd
}

func newMigSpecCmd(stderr io.Writer) *cobra.Command {
	specCmd := &cobra.Command{Use: "spec", Short: "Manage mig specs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	specCmd.AddCommand(&cobra.Command{Use: "set <mig-id|name> <path|->", Short: "Set a mig's spec from a file", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunSpecSet(context.Background(), args[0], args[1], stderr)
	}})
	return specCmd
}

func newMigRepoCmd(stderr io.Writer) *cobra.Command {
	repoCmd := &cobra.Command{Use: "repo", Short: "Manage repos in a mig", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	repoCmd.AddCommand(newMigRepoAddCmd(stderr))
	repoCmd.AddCommand(&cobra.Command{Use: "list <mig-id|name>", Short: "List repos in the mig", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRepoList(context.Background(), args[0], stderr)
	}})
	repoCmd.AddCommand(newMigRepoRemoveCmd(stderr))
	repoCmd.AddCommand(newMigRepoImportCmd(stderr))
	return repoCmd
}

func newMigRepoAddCmd(stderr io.Writer) *cobra.Command {
	var repoURL, baseRef string
	cmd := &cobra.Command{Use: "add <mig-id|name> --repo <url> --base-ref <ref>", Short: "Add a repo to the mig", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRepoAdd(context.Background(), mig.RepoAddOptions{
			MigRef:  args[0],
			RepoURL: repoURL,
			BaseRef: baseRef,
			Output:  stderr,
		})
	}}
	cmd.Flags().StringVar(&repoURL, "repo", "", "Git repository URL")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "Base git ref")
	return cmd
}

func newMigRepoRemoveCmd(stderr io.Writer) *cobra.Command {
	var repoID string
	cmd := &cobra.Command{Use: "remove <mig-id|name> --repo-id <id>", Short: "Remove a repo from the mig", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRepoRemove(context.Background(), args[0], repoID, stderr)
	}}
	cmd.Flags().StringVar(&repoID, "repo-id", "", "Repo ID to remove")
	return cmd
}

func newMigRepoImportCmd(stderr io.Writer) *cobra.Command {
	var file string
	cmd := &cobra.Command{Use: "import <mig-id|name> --file <path>", Short: "Import repos from CSV", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRepoImport(context.Background(), args[0], file, stderr)
	}}
	cmd.Flags().StringVar(&file, "file", "", "Path to CSV file")
	return cmd
}

func newMigPullCmd(stderr io.Writer) *cobra.Command {
	var origin string
	var dryRun, lastFailed, lastSucceeded bool
	cmd := &cobra.Command{Use: "pull [<mig-id|name>]", Short: "Pull diffs into the current git worktree", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		runArgs := []string{"pull"}
		runArgs = addChangedString(cmd, runArgs, "origin", origin)
		runArgs = addChangedBool(cmd, runArgs, "dry-run", dryRun)
		runArgs = addChangedBool(cmd, runArgs, "last-failed", lastFailed)
		runArgs = addChangedBool(cmd, runArgs, "last-succeeded", lastSucceeded)
		runArgs = append(runArgs, args...)
		return pull.HandleMigPull(runArgs[1:], stderr)
	}}
	cmd.Flags().StringVar(&origin, "origin", "origin", "Git remote to match")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and print actions without mutating the repo")
	cmd.Flags().BoolVar(&lastFailed, "last-failed", false, "Select the latest failed run")
	cmd.Flags().BoolVar(&lastSucceeded, "last-succeeded", false, "Select the latest succeeded run")
	return cmd
}

func newMigRunCmd(stderr io.Writer) *cobra.Command {
	var repos []string
	var failed, follow, cancelOnCap bool
	var capDuration time.Duration
	var maxRetries int
	runCmd := &cobra.Command{Use: "run <mig-id|name>", Short: "Run a mig project", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunProject(context.Background(), mig.RunOptions{
			MigRef:      args[0],
			RepoURLs:    repos,
			Failed:      failed,
			Follow:      follow,
			Cap:         capDuration,
			CancelOnCap: cancelOnCap,
			MaxRetries:  maxRetries,
			Output:      stderr,
		})
	}}
	runCmd.Flags().StringArrayVar(&repos, "repo", nil, "Explicit repo URL(s) to run")
	runCmd.Flags().BoolVar(&failed, "failed", false, "Run repos with last terminal state Fail")
	runCmd.Flags().BoolVar(&follow, "follow", false, "Follow run until completion")
	runCmd.Flags().DurationVar(&capDuration, "cap", 0, "Optional time cap for --follow")
	runCmd.Flags().BoolVar(&cancelOnCap, "cancel-on-cap", false, "Cancel run if cap exceeded")
	runCmd.Flags().IntVar(&maxRetries, "max-retries", 5, "Max SSE reconnect attempts")
	runCmd.AddCommand(newMigRunRepoCmd(stderr))
	return runCmd
}

func newMigRunRepoCmd(stderr io.Writer) *cobra.Command {
	repoCmd := &cobra.Command{Use: "repo", Short: "Manage repos within a batch run", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	repoCmd.AddCommand(newMigRunRepoAddCmd(stderr))
	repoCmd.AddCommand(newMigRunRepoRemoveCmd(stderr))
	repoCmd.AddCommand(newMigRunRepoRestartCmd(stderr))
	return repoCmd
}

func newMigRunRepoAddCmd(stderr io.Writer) *cobra.Command {
	var repoURL, baseRef string
	cmd := &cobra.Command{Use: "add --repo-url <url> --base-ref <ref> <run-id>", Short: "Add a repo to a batch run", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRunRepoAdd(context.Background(), mig.RunRepoAddOptions{
			RunID:   args[0],
			RepoURL: repoURL,
			BaseRef: baseRef,
			Output:  stderr,
		})
	}}
	cmd.Flags().StringVar(&repoURL, "repo-url", "", "Git repository URL")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "Git base ref")
	return cmd
}

func newMigRunRepoRemoveCmd(stderr io.Writer) *cobra.Command {
	var repoID string
	cmd := &cobra.Command{Use: "remove --repo-id <id> <run-id>", Short: "Remove/cancel a repo from a batch run", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRunRepoRemove(context.Background(), args[0], repoID, stderr)
	}}
	cmd.Flags().StringVar(&repoID, "repo-id", "", "Repo identifier to remove")
	return cmd
}

func newMigRunRepoRestartCmd(stderr io.Writer) *cobra.Command {
	var repoID, baseRef string
	cmd := &cobra.Command{Use: "restart --repo-id <id> <run-id>", Short: "Restart a repo within a batch run", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRunRepoRestart(context.Background(), mig.RunRepoRestartOptions{
			RunID:   args[0],
			RepoID:  repoID,
			BaseRef: baseRef,
			Output:  stderr,
		})
	}}
	cmd.Flags().StringVar(&repoID, "repo-id", "", "Repo identifier to restart")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "Optional new base ref")
	return cmd
}

func newMigFetchCmd(stderr io.Writer) *cobra.Command {
	var runID, artifactDir string
	cmd := &cobra.Command{Use: "fetch --run <run-id> --artifact-dir <path>", Short: "Download run artifacts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunFetch(context.Background(), runID, artifactDir, stderr)
	}}
	cmd.Flags().StringVar(&runID, "run", "", "Migs run id to fetch artifacts for")
	cmd.Flags().StringVar(&artifactDir, "artifact-dir", "", "Directory to download artifacts into")
	return cmd
}

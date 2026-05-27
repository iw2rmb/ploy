package app

import (
	"context"
	"io"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/mig"
	"github.com/iw2rmb/ploy/internal/cli/pull"
	runcli "github.com/iw2rmb/ploy/internal/cli/run"
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

// newRunCmd creates the cobra command for 'ploy run' (inspect/follow runs).
func newRunCmd(stderr io.Writer) *cobra.Command {
	return newRunCommand(stderr)
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
	var repoURL, baseRef, targetRef string
	cmd := &cobra.Command{Use: "add <mig-id|name> --repo <url> --base-ref <ref> --target-ref <ref>", Short: "Add a repo to the mig", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRepoAdd(context.Background(), mig.RepoAddOptions{
			MigRef:    args[0],
			RepoURL:   repoURL,
			BaseRef:   baseRef,
			TargetRef: targetRef,
			Output:    stderr,
		})
	}}
	cmd.Flags().StringVar(&repoURL, "repo", "", "Git repository URL")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "Base git ref")
	cmd.Flags().StringVar(&targetRef, "target-ref", "", "Target git ref")
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
	var repoURL, baseRef, targetRef string
	cmd := &cobra.Command{Use: "add --repo-url <url> --base-ref <ref> --target-ref <ref> <run-id>", Short: "Add a repo to a batch run", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRunRepoAdd(context.Background(), mig.RunRepoAddOptions{
			RunID:     args[0],
			RepoURL:   repoURL,
			BaseRef:   baseRef,
			TargetRef: targetRef,
			Output:    stderr,
		})
	}}
	cmd.Flags().StringVar(&repoURL, "repo-url", "", "Git repository URL")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "Git base ref")
	cmd.Flags().StringVar(&targetRef, "target-ref", "", "Git target ref")
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
	var repoID, baseRef, targetRef string
	cmd := &cobra.Command{Use: "restart --repo-id <id> <run-id>", Short: "Restart a repo within a batch run", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return mig.RunRunRepoRestart(context.Background(), mig.RunRepoRestartOptions{
			RunID:     args[0],
			RepoID:    repoID,
			BaseRef:   baseRef,
			TargetRef: targetRef,
			Output:    stderr,
		})
	}}
	cmd.Flags().StringVar(&repoID, "repo-id", "", "Repo identifier to restart")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "Optional new base ref")
	cmd.Flags().StringVar(&targetRef, "target-ref", "", "Optional new target ref")
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

func newRunCommand(stderr io.Writer) *cobra.Command {
	var repoURL, baseRef, targetRef, specFile, jobImage, jobCommand, artifactDir string
	var jobEnv []string
	var follow, cancelOnCap, jsonOut bool
	var capDuration time.Duration
	var maxRetries int
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Inspect runs and stream events",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("repo") && !cmd.Flags().Changed("base-ref") && !cmd.Flags().Changed("target-ref") && !cmd.Flags().Changed("spec") {
				return cmd.Help()
			}
			runArgs := []string{}
			runArgs = addChangedString(cmd, runArgs, "repo", repoURL)
			runArgs = addChangedString(cmd, runArgs, "base-ref", baseRef)
			runArgs = addChangedString(cmd, runArgs, "target-ref", targetRef)
			runArgs = addChangedString(cmd, runArgs, "spec", specFile)
			runArgs = addChangedBool(cmd, runArgs, "follow", follow)
			runArgs = addChangedDuration(cmd, runArgs, "cap", capDuration)
			runArgs = addChangedBool(cmd, runArgs, "cancel-on-cap", cancelOnCap)
			runArgs = addChangedInt(cmd, runArgs, "max-retries", maxRetries)
			runArgs = addChangedStringArray(cmd, runArgs, "job-env", jobEnv)
			runArgs = addChangedString(cmd, runArgs, "job-image", jobImage)
			runArgs = addChangedString(cmd, runArgs, "job-command", jobCommand)
			runArgs = addChangedString(cmd, runArgs, "artifact-dir", artifactDir)
			runArgs = addChangedBool(cmd, runArgs, "json", jsonOut)
			return runcli.Handle(runArgs, stderr)
		},
	}
	runCmd.Flags().StringVar(&repoURL, "repo", "", "Git repository URL")
	runCmd.Flags().StringVar(&baseRef, "base-ref", "", "Base Git ref")
	runCmd.Flags().StringVar(&targetRef, "target-ref", "", "Target Git ref")
	runCmd.Flags().StringVar(&specFile, "spec", "", "Path to YAML/JSON spec file")
	runCmd.Flags().BoolVar(&follow, "follow", false, "Follow run until completion")
	runCmd.Flags().DurationVar(&capDuration, "cap", 0, "Optional time cap for --follow")
	runCmd.Flags().BoolVar(&cancelOnCap, "cancel-on-cap", false, "Cancel run if cap exceeded")
	runCmd.Flags().IntVar(&maxRetries, "max-retries", 5, "Max report fetch retries")
	runCmd.Flags().StringArrayVar(&jobEnv, "job-env", nil, "Job environment KEY=VALUE")
	runCmd.Flags().StringVar(&jobImage, "job-image", "", "Container image for the mig step")
	runCmd.Flags().StringVar(&jobCommand, "job-command", "", "Container command override")
	runCmd.Flags().StringVar(&artifactDir, "artifact-dir", "", "Directory to download final artifacts into")
	runCmd.Flags().BoolVar(&jsonOut, "json", false, "Print machine-readable JSON summary")
	runCmd.AddCommand(newRunListCmd(stderr))
	runCmd.AddCommand(newRunCancelCmd(stderr))
	runCmd.AddCommand(&cobra.Command{Use: "start <run-id>", Short: "Start pending repos for a batch run", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return runcli.Handle(append([]string{"start"}, args...), stderr)
	}})
	runCmd.AddCommand(newRunStatusCmd(stderr))
	runCmd.AddCommand(newRunLogsCmd(stderr))
	runCmd.AddCommand(newRunPullCmd(stderr))
	runCmd.AddCommand(newRunPatchCmd(stderr))
	return runCmd
}

func newRunListCmd(stderr io.Writer) *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{Use: "ls", Short: "List batch runs with pagination", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		runArgs := []string{"ls"}
		runArgs = addChangedInt(cmd, runArgs, "limit", limit)
		runArgs = addChangedInt(cmd, runArgs, "offset", offset)
		return runcli.Handle(runArgs, stderr)
	}}
	cmd.Flags().IntVar(&limit, "limit", 50, "Max number of runs to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "Number of runs to skip")
	return cmd
}

func newRunCancelCmd(stderr io.Writer) *cobra.Command {
	var reason string
	cmd := &cobra.Command{Use: "cancel [--reason <text>] <run-id>", Short: "Cancel a run via the control plane", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		runArgs := []string{"cancel"}
		runArgs = addChangedString(cmd, runArgs, "reason", reason)
		runArgs = append(runArgs, args...)
		return runcli.Handle(runArgs, stderr)
	}}
	cmd.Flags().StringVar(&reason, "reason", "", "Optional reason for cancellation")
	return cmd
}

func newRunStatusCmd(stderr io.Writer) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{Use: "status [--json] <run-id>", Short: "Show status for a run", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		runArgs := []string{"status"}
		runArgs = addChangedBool(cmd, runArgs, "json", jsonOut)
		runArgs = append(runArgs, args...)
		return runcli.Handle(runArgs, stderr)
	}}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print machine-readable JSON report")
	return cmd
}

func newRunLogsCmd(stderr io.Writer) *cobra.Command {
	var maxRetries int
	var idleTimeout, timeout time.Duration
	cmd := &cobra.Command{Use: "logs <run-id>", Short: "Stream run lifecycle events", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		runArgs := []string{"logs"}
		runArgs = addChangedInt(cmd, runArgs, "max-retries", maxRetries)
		runArgs = addChangedDuration(cmd, runArgs, "idle-timeout", idleTimeout)
		runArgs = addChangedDuration(cmd, runArgs, "timeout", timeout)
		runArgs = append(runArgs, args...)
		return runcli.Handle(runArgs, stderr)
	}}
	cmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Max reconnect attempts")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout", 45*time.Second, "Cancel if no events arrive")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Overall timeout for the stream")
	return cmd
}

func newRunPullCmd(stderr io.Writer) *cobra.Command {
	var origin string
	var dryRun bool
	cmd := &cobra.Command{Use: "pull [--origin <remote>] [--dry-run] <run-id>", Short: "Pull diffs into the current git worktree", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		runArgs := []string{"pull"}
		runArgs = addChangedString(cmd, runArgs, "origin", origin)
		runArgs = addChangedBool(cmd, runArgs, "dry-run", dryRun)
		runArgs = append(runArgs, args...)
		return runcli.Handle(runArgs, stderr)
	}}
	cmd.Flags().StringVar(&origin, "origin", "origin", "Git remote to match")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and print actions without mutating the repo")
	return cmd
}

func newRunPatchCmd(stderr io.Writer) *cobra.Command {
	var repoID, repoURL, diffID, output string
	cmd := &cobra.Command{Use: "patch [--repo-id <id> | --repo-url <url>] [--diff-id <uuid>] [--output <path|->] <run-id>", Short: "Download a run patch artifact", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		runArgs := []string{"patch"}
		runArgs = addChangedString(cmd, runArgs, "repo-id", repoID)
		runArgs = addChangedString(cmd, runArgs, "repo-url", repoURL)
		runArgs = addChangedString(cmd, runArgs, "diff-id", diffID)
		runArgs = addChangedString(cmd, runArgs, "output", output)
		runArgs = append(runArgs, args...)
		return runcli.Handle(runArgs, stderr)
	}}
	cmd.Flags().StringVar(&repoID, "repo-id", "", "Repo id")
	cmd.Flags().StringVar(&repoURL, "repo-url", "", "Repo url")
	cmd.Flags().StringVar(&diffID, "diff-id", "", "Specific diff id to download")
	cmd.Flags().StringVar(&output, "output", "-", "Output path")
	return cmd
}

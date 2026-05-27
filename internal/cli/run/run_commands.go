package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	pullcli "github.com/iw2rmb/ploy/internal/cli/pull"
	runcmd "github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/spf13/cobra"
)

// NewCommand constructs the Cobra command tree for `ploy run`.
func NewCommand() *cobra.Command {
	submit := SubmitOptions{MaxRetries: 5}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Inspect runs and stream events",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("repo") &&
				!cmd.Flags().Changed("base-ref") &&
				!cmd.Flags().Changed("target-ref") &&
				!cmd.Flags().Changed("spec") {
				return cmd.Help()
			}
			submit.Output = cmd.OutOrStdout()
			submit.FollowOutput = cmd.ErrOrStderr()
			return RunSubmit(cmd.Context(), submit)
		},
	}

	cmd.Flags().StringVar(&submit.RepoURL, "repo", "", "Git repository URL")
	cmd.Flags().StringVar(&submit.BaseRef, "base-ref", "", "Base Git ref")
	cmd.Flags().StringVar(&submit.TargetRef, "target-ref", "", "Target Git ref")
	cmd.Flags().StringVar(&submit.SpecFile, "spec", "", "Path to YAML/JSON spec file")
	cmd.Flags().BoolVar(&submit.Follow, "follow", false, "Follow run until completion")
	cmd.Flags().DurationVar(&submit.CapDuration, "cap", 0, "Optional time cap for --follow")
	cmd.Flags().BoolVar(&submit.CancelOnCap, "cancel-on-cap", false, "Cancel run if cap exceeded")
	cmd.Flags().IntVar(&submit.MaxRetries, "max-retries", 5, "Max report fetch retries")
	cmd.Flags().StringArrayVar(&submit.MigEnvs, "job-env", nil, "Job environment KEY=VALUE")
	cmd.Flags().StringVar(&submit.JobImage, "job-image", "", "Container image for the mig step")
	cmd.Flags().StringVar(&submit.MigCommand, "job-command", "", "Container command override")
	cmd.Flags().StringVar(&submit.ArtifactDir, "artifact-dir", "", "Directory to download final artifacts into")
	cmd.Flags().BoolVar(&submit.JSONOut, "json", false, "Print machine-readable JSON summary")

	cmd.AddCommand(newListCommand())
	cmd.AddCommand(newCancelCommand())
	cmd.AddCommand(newStartCommand())
	cmd.AddCommand(newStatusCommand())
	cmd.AddCommand(newLogsCommand())
	cmd.AddCommand(newPullCommand())
	cmd.AddCommand(newPatchCommand())
	return cmd
}

func newListCommand() *cobra.Command {
	opts := ListOptions{Limit: 50}
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List batch runs with pagination",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Output = cmd.OutOrStdout()
			return RunList(cmd.Context(), opts)
		},
	}
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Max number of runs to return")
	cmd.Flags().IntVar(&opts.Offset, "offset", 0, "Number of runs to skip")
	return cmd
}

func newCancelCommand() *cobra.Command {
	opts := CancelOptions{}
	cmd := &cobra.Command{
		Use:   "cancel [--reason <text>] <run-id>",
		Short: "Cancel a run via the control plane",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RunID = args[0]
			opts.Output = cmd.OutOrStdout()
			return RunCancel(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Reason, "reason", "", "Optional reason for cancellation")
	return cmd
}

func newStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start <run-id>",
		Short: "Start pending repos for a batch run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunStart(cmd.Context(), StartOptions{
				RunID:  args[0],
				Output: cmd.OutOrStdout(),
			})
		},
	}
}

func newStatusCommand() *cobra.Command {
	opts := StatusOptions{}
	cmd := &cobra.Command{
		Use:   "status [--json] <run-id>",
		Short: "Show status for a run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RunID = args[0]
			opts.Output = cmd.OutOrStdout()
			return RunStatus(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.JSONOut, "json", false, "Print machine-readable JSON report")
	return cmd
}

func newLogsCommand() *cobra.Command {
	opts := LogsOptions{
		MaxRetries:  3,
		IdleTimeout: 45 * time.Second,
	}
	cmd := &cobra.Command{
		Use:   "logs <run-id>",
		Short: "Stream run lifecycle events",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RunID = args[0]
			opts.Output = cmd.OutOrStdout()
			return RunLogs(cmd.Context(), opts)
		},
	}
	cmd.Flags().IntVar(&opts.MaxRetries, "max-retries", 3, "Max reconnect attempts")
	cmd.Flags().DurationVar(&opts.IdleTimeout, "idle-timeout", 45*time.Second, "Cancel if no events arrive")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 0, "Overall timeout for the stream")
	return cmd
}

func newPullCommand() *cobra.Command {
	var origin string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "pull [--origin <remote>] [--dry-run] <run-id>",
		Short: "Pull diffs into the current git worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := []string{}
			if cmd.Flags().Changed("origin") {
				runArgs = append(runArgs, "--origin", origin)
			}
			if cmd.Flags().Changed("dry-run") {
				runArgs = append(runArgs, fmt.Sprintf("--dry-run=%t", dryRun))
			}
			runArgs = append(runArgs, args...)
			return pullcli.HandleRunPull(runArgs, cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&origin, "origin", "origin", "Git remote to match")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and print actions without mutating the repo")
	return cmd
}

func newPatchCommand() *cobra.Command {
	opts := PatchOptions{OutputPath: "-"}
	cmd := &cobra.Command{
		Use:   "patch [--repo-id <id> | --repo-url <url>] [--diff-id <uuid>] [--output <path|->] <run-id>",
		Short: "Download a run patch artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RunID = args[0]
			opts.Output = cmd.OutOrStdout()
			return RunPatch(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.RepoID, "repo-id", "", "Repo id")
	cmd.Flags().StringVar(&opts.RepoURL, "repo-url", "", "Repo url")
	cmd.Flags().StringVar(&opts.DiffID, "diff-id", "", "Specific diff id to download")
	cmd.Flags().StringVar(&opts.OutputPath, "output", "-", "Output path")
	return cmd
}

type StatusOptions struct {
	RunID   string
	JSONOut bool
	Output  io.Writer
}

func RunStatus(ctx context.Context, opts StatusOptions) error {
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		return errors.New("run id required")
	}
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	report, err := runcmd.GetRunReportCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   domaintypes.RunID(runID),
	}.Run(ctx)
	if err != nil {
		return err
	}

	if opts.JSONOut {
		return runcmd.RenderRunReportJSON(out, report)
	}

	token, err := common.ResolveControlPlaneToken()
	if err != nil {
		return err
	}
	return runcmd.RenderRunStatusSnapshotText(out, report, runcmd.TextRenderOptions{
		EnableOSC8: common.SupportsOSC8(out),
		AuthToken:  token,
		BaseURL:    base,
	})
}

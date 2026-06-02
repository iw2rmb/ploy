package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	runcmd "github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/spf13/cobra"
)

// NewCommand constructs the Cobra command tree for `ploy run`.
func NewCommand() *cobra.Command {
	submit := SubmitOptions{MaxRetries: 5}
	cmd := &cobra.Command{
		Use:   "run <spec-path> [<repo-path>|<namespace/repo[:ref]>]",
		Short: "Submit and inspect runs",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			submit.SpecPath = args[0]
			if len(args) > 1 {
				submit.RepoSelector = args[1]
			}
			submit.PullArtifacts = cmd.Flags().Changed("pull")
			submit.Output = cmd.OutOrStdout()
			submit.FollowOutput = cmd.ErrOrStderr()
			return RunSubmit(cmd.Context(), submit)
		},
	}

	cmd.Flags().BoolVar(&submit.Apply, "apply", false, "Apply the resulting patch to a local repo after success")
	cmd.Flags().StringVar(&submit.PullPath, "pull", "", "Download final artifacts after success; optional path")
	if flag := cmd.Flags().Lookup("pull"); flag != nil {
		flag.NoOptDefVal = osTempArtifactDirSentinel
	}

	cmd.AddCommand(newListCommand())
	cmd.AddCommand(newCancelCommand())
	cmd.AddCommand(newRestartCommand())
	cmd.AddCommand(newStatusCommand())
	cmd.AddCommand(newSBOMCommand())
	cmd.AddCommand(newPullCommand())
	cmd.AddCommand(newApplyCommand())
	return cmd
}

func newListCommand() *cobra.Command {
	opts := ListOptions{Limit: 50}
	cmd := &cobra.Command{
		Use:   "ls [<path>|<namespace/repo[:ref]>]",
		Short: "List runs with pagination",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoSelector = args[0]
			}
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
		Use:   "cancel <run-id>",
		Short: "Cancel a run via the control plane",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RunID = args[0]
			opts.Output = cmd.OutOrStdout()
			return RunCancel(cmd.Context(), opts)
		},
	}
	return cmd
}

func newRestartCommand() *cobra.Command {
	opts := RestartOptions{}
	cmd := &cobra.Command{
		Use:   "restart <run-id>",
		Short: "Restart a terminal run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RunID = args[0]
			opts.Output = cmd.OutOrStdout()
			return RunRestart(cmd.Context(), opts)
		},
	}
	return cmd
}

func newStatusCommand() *cobra.Command {
	opts := StatusOptions{}
	cmd := &cobra.Command{
		Use:   "status <run-id>",
		Short: "Show status for a run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RunID = args[0]
			opts.Output = cmd.OutOrStdout()
			return RunStatus(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.JSONOut, "json", false, "Print machine-readable JSON report")
	cmd.Flags().BoolVar(&opts.Follow, "follow", false, "Follow run status until completion")
	return cmd
}

func newSBOMCommand() *cobra.Command {
	opts := SBOMOptions{}
	cmd := &cobra.Command{
		Use:          "sbom {pre|post|diff} <run-id>",
		Short:        "Show persisted run SBOM packages",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.View = args[0]
			opts.RunID = args[1]
			opts.Output = cmd.OutOrStdout()
			return RunSBOM(cmd.Context(), opts)
		},
	}
	return cmd
}

func newPullCommand() *cobra.Command {
	opts := ArtifactPullOptions{}
	cmd := &cobra.Command{
		Use:   "pull <run-id> [artifacts-path]",
		Short: "Download final run artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RunID = args[0]
			if len(args) > 1 {
				opts.ArtifactsPath = args[1]
			}
			opts.Output = cmd.OutOrStdout()
			return RunArtifactPull(cmd.Context(), opts)
		},
	}
	return cmd
}

func newApplyCommand() *cobra.Command {
	opts := ApplyOptions{}
	cmd := &cobra.Command{
		Use:   "apply <run-id> [path]",
		Short: "Apply a successful run result to a local repo",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RunID = args[0]
			if len(args) > 1 {
				opts.RepoPath = args[1]
			}
			opts.Output = cmd.OutOrStdout()
			return RunApply(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Apply even if local HEAD differs from the run source SHA")
	return cmd
}

type StatusOptions struct {
	RunID   string
	JSONOut bool
	Follow  bool
	Output  io.Writer
}

type SBOMOptions struct {
	View   string
	RunID  string
	Output io.Writer
}

func RunSBOM(ctx context.Context, opts SBOMOptions) error {
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
	result, err := runcmd.GetRunSBOMCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   domaintypes.RunID(runID),
		View:    opts.View,
	}.Run(ctx)
	if err != nil {
		return err
	}
	return runcmd.RenderRunSBOM(out, result)
}

func RunStatus(ctx context.Context, opts StatusOptions) error {
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		return errors.New("run id required")
	}
	if opts.JSONOut && opts.Follow {
		return errors.New("--json and --follow are mutually exclusive")
	}
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	report, err := runcmd.GetRunStatusReportCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   domaintypes.RunID(runID),
	}.Run(ctx)
	if err != nil {
		return err
	}

	if opts.JSONOut {
		return runcmd.RenderRunStatusReportJSON(out, report)
	}

	token, err := common.ResolveControlPlaneToken()
	if err != nil {
		return err
	}
	if opts.Follow {
		final, err := followRunStatusReports(ctx, base, httpClient, domaintypes.RunID(runID), out, 5, time.Second)
		if err != nil {
			return err
		}
		if final != "" && final != "succeeded" {
			return fmt.Errorf("run ended in %s", strings.ToLower(string(final)))
		}
		return nil
	}
	return runcmd.RenderRunStatusSnapshotText(out, report, runcmd.TextRenderOptions{
		EnableOSC8: common.SupportsOSC8(out),
		AuthToken:  token,
		BaseURL:    base,
	})
}

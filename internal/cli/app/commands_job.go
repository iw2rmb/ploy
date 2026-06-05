package app

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/job"
	"github.com/spf13/cobra"
)

// newJobCmd creates the cobra command tree for 'ploy job' and its subcommands.
func newJobCmd(stdout, stderr io.Writer) *cobra.Command {
	jobCmd := &cobra.Command{
		Use:   "job",
		Short: "Inspect jobs and follow logs",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	jobCmd.AddCommand(newJobStatusCmd(stdout))
	jobCmd.AddCommand(newJobLogCmd(stderr))
	return jobCmd
}

func newJobStatusCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <job-id>",
		Short: "Print job status as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return job.RunStatus(cmd.Context(), job.StatusOptions{
				JobID:  args[0],
				Output: stdout,
			})
		},
	}
	return cmd
}

func newJobLogCmd(stderr io.Writer) *cobra.Command {
	var format string
	var follow bool
	var maxRetries int
	var idleTimeout, timeout time.Duration
	cmd := &cobra.Command{
		Use:   "log [--follow|-f] [--format <raw|structured>] <job-id>",
		Short: "Print job logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := job.RunLog(context.Background(), job.LogOptions{
				JobID:       args[0],
				Format:      format,
				Follow:      follow,
				MaxRetries:  maxRetries,
				IdleTimeout: idleTimeout,
				Timeout:     timeout,
				Output:      stderr,
			})
			if errors.Is(err, job.ErrInvalidFormat) {
				_ = cmd.Help()
			}
			return err
		},
	}
	cmd.Flags().StringVar(&format, "format", "raw", "Output format (raw|structured)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow stream after printing available logs")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Max reconnect attempts")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout", 45*time.Second, "Cancel if no events arrive")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Overall timeout for the stream")
	return cmd
}

package main

import (
	"io"

	"github.com/spf13/cobra"
)

// newJobCmd creates the cobra command tree for 'ploy job' and its subcommands.
func newJobCmd(stderr io.Writer) *cobra.Command {
	jobCmd := &cobra.Command{
		Use:                "job",
		Short:              "Inspect and follow job logs",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleJob(args, stderr)
		},
	}
	return jobCmd
}

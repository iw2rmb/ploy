package main

import (
	"io"

	"github.com/spf13/cobra"
)

func newRerunCmd(stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:                "rerun",
		Short:              "Rerun a job with alter overlays",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleRerun(args, stderr)
		},
	}
}

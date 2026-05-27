package app

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func addChangedString(cmd *cobra.Command, args []string, name string, value string) []string {
	if cmd.Flags().Changed(name) {
		args = append(args, "--"+name, value)
	}
	return args
}

func addChangedInt(cmd *cobra.Command, args []string, name string, value int) []string {
	if cmd.Flags().Changed(name) {
		args = append(args, "--"+name, fmt.Sprintf("%d", value))
	}
	return args
}

func addChangedDuration(cmd *cobra.Command, args []string, name string, value time.Duration) []string {
	if cmd.Flags().Changed(name) {
		args = append(args, "--"+name, value.String())
	}
	return args
}

func addChangedBool(cmd *cobra.Command, args []string, name string, value bool) []string {
	if cmd.Flags().Changed(name) {
		args = append(args, fmt.Sprintf("--%s=%t", name, value))
	}
	return args
}

func addChangedStringArray(cmd *cobra.Command, args []string, name string, values []string) []string {
	if cmd.Flags().Changed(name) {
		for _, value := range values {
			args = append(args, "--"+name, value)
		}
	}
	return args
}

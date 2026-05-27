package configure

import (
	"io"

	"github.com/spf13/cobra"
)

// NewCommand constructs the cobra command tree for `ploy config`.
func NewCommand(stdout, stderr io.Writer) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect or update cluster configuration",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	if stdout != nil {
		configCmd.SetOut(stdout)
	}
	if stderr != nil {
		configCmd.SetErr(stderr)
	}
	configCmd.AddCommand(newEnvCommand())
	return configCmd
}

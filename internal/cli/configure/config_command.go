package configure

import (
	"errors"
	"fmt"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"io"
)

// Handle routes config subcommands.
func Handle(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if common.WantsHelp(args) {
		printConfigUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printConfigUsage(stderr)
		return errors.New("config subcommand required")
	}
	switch args[0] {
	case "env":
		return handleConfigEnv(args[1:], stderr)
	default:
		printConfigUsage(stderr)
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func printConfigUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  env       Manage global environment variables")
}

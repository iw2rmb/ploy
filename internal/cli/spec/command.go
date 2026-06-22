package spec

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/specpayload"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Handle routes spec subcommands.
func Handle(args []string, stdout, stderr io.Writer) error {
	if common.WantsHelp(args) {
		printSpecUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printSpecUsage(stderr)
		return errors.New("spec subcommand required")
	}

	switch args[0] {
	case "schema":
		return handleSchema(args[1:], stdout, stderr)
	case "validate":
		return handleValidate(args[1:], stderr)
	case "push":
		return handlePush(args[1:], stdout, stderr)
	case "ls":
		return handleList(args[1:], stdout, stderr)
	default:
		action, ok, err := parseSpecArchiveArgs(args)
		if err != nil {
			return err
		}
		if ok {
			return handleArchiveAction(action, stdout, stderr)
		}
		return fmt.Errorf("unknown spec subcommand %q", args[0])
	}
}

func printSpecUsage(w io.Writer) {
	common.PrintCommandUsage(w, "spec")
}

func handleSchema(args []string, stdout, stderr io.Writer) error {
	if common.WantsHelp(args) {
		printSchemaUsage(stderr)
		return nil
	}
	if len(args) > 0 {
		printSchemaUsage(stderr)
		return fmt.Errorf("spec schema takes no arguments")
	}
	data, err := contracts.MigSpecSchemaJSON()
	if err != nil {
		return err
	}
	if _, err := stdout.Write(data); err != nil {
		return fmt.Errorf("write spec schema: %w", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		if _, err := fmt.Fprintln(stdout); err != nil {
			return fmt.Errorf("write spec schema newline: %w", err)
		}
	}
	return nil
}

func printSchemaUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy spec schema")
}

func handleValidate(args []string, stderr io.Writer) error {
	if common.WantsHelp(args) {
		printValidateUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printValidateUsage(stderr)
		return errors.New("spec path required")
	}
	for _, path := range args {
		path = strings.TrimSpace(path)
		if path == "" {
			return errors.New("spec path required")
		}
		if _, err := specpayload.ValidateLocalFile(path); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		_, _ = fmt.Fprintf(stderr, "Validated spec %s\n", path)
	}
	return nil
}

func printValidateUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy spec validate <path> [<path>...]")
}

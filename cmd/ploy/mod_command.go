package main

import (
	"errors"
	"fmt"
	"io"
)

// handleMod routes Mods subcommands to their implementations.
func handleMod(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printModUsage(stderr)
		return errors.New("mod subcommand required")
	}

	switch args[0] {
	case "run":
		return handleModRun(args[1:], stderr)
	default:
		printModUsage(stderr)
		return fmt.Errorf("unknown mod subcommand %q", args[0])
	}
}

func printModUsage(w io.Writer) {
	printCommandUsage(w, "mod")
}

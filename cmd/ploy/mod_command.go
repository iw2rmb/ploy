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
	lines := []string{
		"Usage: ploy mod <command>",
		"",
		"Commands:",
		"  plan      Generate a Mods plan locally before submission",
		"  run       Submit a Mods run to the control plane",
		"  resume    Resume a paused or interrupted Mods run",
		"  inspect   Show Mods ticket status and artifacts",
		"",
		"Use 'ploy help mod <command>' for command-specific details.",
	}
	for _, line := range lines {
		_, _ = fmt.Fprintln(w, line)
	}
}

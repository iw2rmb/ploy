package main

import (
	"errors"
	"fmt"
	"io"
)

// handleWorkflow routes workflow subcommands to their implementations.
func handleWorkflow(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printWorkflowUsage(stderr)
		return errors.New("workflow subcommand required")
	}

	switch args[0] {
	case "run":
		return handleWorkflowRun(args[1:], stderr)
	case "cancel":
		return handleWorkflowCancel(args[1:], stderr)
	default:
		printWorkflowUsage(stderr)
		return fmt.Errorf("unknown workflow subcommand %q", args[0])
	}
}

// printWorkflowUsage details the workflow command usage information.
func printWorkflowUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  run    Consume a ticket and execute the workflow (stub)")
	_, _ = fmt.Fprintln(w, "  cancel Cancel an in-flight workflow run")
}

// printWorkflowRunUsage outputs the accepted workflow run flags.
func printWorkflowRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow run --tenant <tenant> [--ticket <ticket-id>|--ticket auto] [--mods-plan-timeout <duration>] [--mods-max-parallel <n>]")
}

// printWorkflowCancelUsage documents the workflow cancellation flags.
func printWorkflowCancelUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow cancel --tenant <tenant> --run-id <run-id> [--workflow <workflow-id>] [--reason <text>]")
}

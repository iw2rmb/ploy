package main

import (
	"errors"
	"fmt"
	"io"
)

// handleRunDiff is a stub for the removed `ploy run diffs` command.
// Run-level diff listing is no longer supported; use repo-scoped pull instead.
func handleRunDiff(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printRunDiffUsage(stderr)
		return nil
	}
	return errors.New("run diffs: this command has been removed; use `ploy run pull <run-id>` or `ploy mig pull`")
}

func printRunDiffUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy run diff (removed)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "This command has been removed; use `ploy run pull <run-id>` or `ploy mig pull`.")
}

package main

import (
	"errors"
	"io"
)

// handleRunDiff is a stub for the removed `ploy run diffs` command.
// Run-level diff listing is no longer supported; use repo-scoped pull instead.
func handleRunDiff(_ []string, stderr io.Writer) error {
	return errors.New("run diffs: this command has been removed; use `ploy run pull <run-id>` or `ploy mig pull`")
}

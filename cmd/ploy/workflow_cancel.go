package main

import (
    "errors"
    "flag"
    "fmt"
    "io"
    "strings"
)

// handleWorkflowCancel is deprecated in favour of `ploy mod cancel`.
func handleWorkflowCancel(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("workflow cancel", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    runID := fs.String("run-id", "", "workflow run identifier to cancel (deprecated)")
    _ = fs.String("workflow", "", "ignored (deprecated)")
    _ = fs.String("reason", "", "ignored (deprecated)")
    if err := fs.Parse(args); err != nil {
        printWorkflowCancelUsage(stderr)
        return err
    }
    if strings.TrimSpace(*runID) == "" {
        printWorkflowCancelUsage(stderr)
        return errors.New("run id required")
    }
    _, _ = fmt.Fprintln(stderr, "Command deprecated: use 'ploy mod cancel --ticket <ticket> [--reason <text>]' instead.")
    return fmt.Errorf("workflow cancel is deprecated")
}

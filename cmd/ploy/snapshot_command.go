package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
)

// handleSnapshot routes snapshot subcommands to their handlers.
func handleSnapshot(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printSnapshotUsage(stderr)
		return errors.New("snapshot subcommand required")
	}

	switch args[0] {
	case "plan":
		return handleSnapshotPlan(args[1:], stderr)
	case "capture":
		return handleSnapshotCapture(args[1:], stderr)
	default:
		printSnapshotUsage(stderr)
		return fmt.Errorf("unknown snapshot subcommand %q", args[0])
	}
}

// printSnapshotUsage lists the available snapshot subcommands.
func printSnapshotUsage(w io.Writer) {
	printCommandUsage(w, "snapshot")
}

// handleSnapshotPlan renders a snapshot plan preview from the registry.
func handleSnapshotPlan(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("snapshot plan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	snapshotName := fs.String("snapshot", "", "snapshot identifier to plan")
	if err := fs.Parse(args); err != nil {
		printSnapshotPlanUsage(stderr)
		return err
	}

	if strings.TrimSpace(*snapshotName) == "" {
		printSnapshotPlanUsage(stderr)
		return errors.New("snapshot is required")
	}

	reg, err := snapshotRegistryLoader(snapshotConfigDir)
	if err != nil {
		return err
	}

	report, err := reg.Plan(context.Background(), strings.TrimSpace(*snapshotName))
	if err != nil {
		return err
	}

	printSnapshotPlan(stderr, report)
	return nil
}

// printSnapshotPlanUsage informs about the required plan flags.
func printSnapshotPlanUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy snapshot plan --snapshot <snapshot-name>")
}

// printSnapshotPlan outputs the main snapshot plan attributes.
func printSnapshotPlan(w io.Writer, report snapshots.PlanReport) {
	_, _ = fmt.Fprintf(w, "Snapshot: %s\n", report.SnapshotName)
	if report.Description != "" {
		_, _ = fmt.Fprintf(w, "Description: %s\n", report.Description)
	}
	_, _ = fmt.Fprintf(w, "Engine: %s\n", report.Engine)
	if report.FixturePath != "" {
		_, _ = fmt.Fprintf(w, "Fixture: %s\n", report.FixturePath)
	}
	_, _ = fmt.Fprintf(w, "Strip Rules: %d\n", report.Stripping.Total)
	_, _ = fmt.Fprintf(w, "Mask Rules: %d\n", report.Masking.Total)
	_, _ = fmt.Fprintf(w, "Synthetic Rules: %d\n", report.Synthetic.Total)
	if len(report.Highlights) > 0 {
		_, _ = fmt.Fprintln(w, "Highlights:")
		for _, highlight := range report.Highlights {
			_, _ = fmt.Fprintf(w, "  - %s\n", highlight)
		}
	}
}

// handleSnapshotCapture launches a snapshot capture and prints the result.
func handleSnapshotCapture(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("snapshot capture", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	snapshotName := fs.String("snapshot", "", "snapshot identifier to capture")
    // tenant removed
	ticket := fs.String("ticket", "", "ticket identifier associated with capture")
	if err := fs.Parse(args); err != nil {
		printSnapshotCaptureUsage(stderr)
		return err
	}

	trimmedSnapshot := strings.TrimSpace(*snapshotName)
	if trimmedSnapshot == "" {
		printSnapshotCaptureUsage(stderr)
		return errors.New("snapshot is required")
	}
    // tenant removed
	trimmedTicket := strings.TrimSpace(*ticket)
	if trimmedTicket == "" {
		printSnapshotCaptureUsage(stderr)
		return errors.New("ticket is required")
	}

	reg, err := snapshotRegistryLoader(snapshotConfigDir)
	if err != nil {
		return err
	}

    result, err := reg.Capture(context.Background(), trimmedSnapshot, snapshots.CaptureOptions{TicketID: trimmedTicket})
	if err != nil {
		return err
	}

	printSnapshotCapture(stderr, result)
	return nil
}

// printSnapshotCaptureUsage displays the capture usage text.
func printSnapshotCaptureUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy snapshot capture --snapshot <snapshot-name> --ticket <ticket-id>")
}

// printSnapshotCapture renders snapshot capture metadata.
func printSnapshotCapture(w io.Writer, result snapshots.CaptureResult) {
	_, _ = fmt.Fprintf(w, "Snapshot: %s\n", result.Metadata.SnapshotName)
	_, _ = fmt.Fprintf(w, "Artifact CID: %s\n", result.ArtifactCID)
	_, _ = fmt.Fprintf(w, "Fingerprint: %s\n", result.Fingerprint)
	_, _ = fmt.Fprintf(w, "Captured At: %s\n", result.Metadata.CapturedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "Rule Totals: strip=%d mask=%d synthetic=%d\n", result.Metadata.RuleCounts.Strip, result.Metadata.RuleCounts.Mask, result.Metadata.RuleCounts.Synthetic)
	if len(result.Diff.StrippedColumns) > 0 {
		_, _ = fmt.Fprintln(w, "Stripped Columns:")
		printColumnSet(w, result.Diff.StrippedColumns)
	}
	if len(result.Diff.MaskedColumns) > 0 {
		_, _ = fmt.Fprintln(w, "Masked Columns:")
		printColumnSet(w, result.Diff.MaskedColumns)
	}
	if len(result.Diff.SyntheticColumns) > 0 {
		_, _ = fmt.Fprintln(w, "Synthetic Columns:")
		printColumnSet(w, result.Diff.SyntheticColumns)
	}
}

// printColumnSet alphabetises and renders column changes by table.
func printColumnSet(w io.Writer, values map[string][]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cols := append([]string(nil), values[key]...)
		sort.Strings(cols)
		_, _ = fmt.Fprintf(w, "  %s: %s\n", key, strings.Join(cols, ", "))
	}
}

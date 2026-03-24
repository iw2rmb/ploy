package main

import (
	"context"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	internaltui "github.com/iw2rmb/ploy/internal/tui"
)

// printTUIUsage prints help for 'ploy tui'.
func printTUIUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy tui")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Launch the interactive terminal UI.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Screens:")
	_, _ = fmt.Fprintln(w, "  PLOY              Root selector: Migrations | Runs | Jobs")
	_, _ = fmt.Fprintln(w, "  PLOY | MIGRATIONS Migration list (newest-to-oldest)")
	_, _ = fmt.Fprintln(w, "  MIGRATION <name>  Migration details: repositories and runs totals")
	_, _ = fmt.Fprintln(w, "  PLOY | RUNS       Run list (newest-to-oldest) with timestamp")
	_, _ = fmt.Fprintln(w, "  RUN               Run details: repositories and jobs totals")
	_, _ = fmt.Fprintln(w, "  PLOY | JOBS       Jobs list: job, mig name, run id, repo id")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Navigation:")
	_, _ = fmt.Fprintln(w, "  Enter  Drill into the selected item")
	_, _ = fmt.Fprintln(w, "  Esc    Return to the previous screen")
	_, _ = fmt.Fprintln(w, "  q      Quit")
}

// newTUICmd creates the cobra command for 'ploy tui'.
func newTUICmd(stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive TUI for migrations, runs, and jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(stderr)
		},
	}
}

// runTUI resolves the control-plane client and launches the Bubble Tea program.
func runTUI(stderr io.Writer) error {
	baseURL, client, err := resolveControlPlaneHTTP(context.Background())
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	m := internaltui.InitialModel(client, baseURL)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		_, _ = fmt.Fprintf(stderr, "tui: %v\n", err)
		return err
	}
	return nil
}

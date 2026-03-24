package main

import (
	"context"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	internaltui "github.com/iw2rmb/ploy/internal/tui"
)

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

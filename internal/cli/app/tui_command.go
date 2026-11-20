package app

import (
	"context"
	"fmt"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	internaltui "github.com/iw2rmb/ploy/internal/tui"
)

// newTUICmd creates the cobra command for 'ploy tui'.
func newTUICmd(stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive TUI for runs and jobs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(cmd.Context(), stderr)
		},
	}
}

// runTUI resolves the control-plane client and launches the Bubble Tea program.
func runTUI(ctx context.Context, stderr io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	baseURL, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	m := internaltui.InitialModel(ctx, client, baseURL)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		_, _ = fmt.Fprintf(stderr, "tui: %v\n", err)
		return err
	}
	return nil
}

package main

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/iw2rmb/ploy/cmd/ploy/config"
)

func handleCluster(args []string, w io.Writer) error {
	if len(args) == 0 {
		printClusterUsage(w)
		return errors.New("cluster subcommand required")
	}
	switch args[0] {
	case "list":
		return handleClusterList(w)
	case "connect":
		return handleClusterConnect(args[1:], w)
	default:
		printClusterUsage(w)
		return fmt.Errorf("unknown cluster subcommand %q", args[0])
	}
}

func handleClusterList(w io.Writer) error {
	descs, err := config.ListDescriptors()
	if err != nil {
		return err
	}
	if len(descs) == 0 {
		_, _ = fmt.Fprintln(w, "No clusters cached. Run 'ploy cluster connect' to add one.")
		return nil
	}
	_, _ = fmt.Fprintf(w, "Clusters (%d):\n", len(descs))
	for _, desc := range descs {
		label := desc.ID
		if desc.Default {
			label += " (default)"
		}
		_, _ = fmt.Fprintf(w, "  - %s  beacon=%s", label, desc.BeaconURL)
		if desc.ControlPlaneURL != "" {
			_, _ = fmt.Fprintf(w, "  control=%s", desc.ControlPlaneURL)
		}
		if desc.Version != "" {
			_, _ = fmt.Fprintf(w, "  version=%s", desc.Version)
		}
		if !desc.LastRefreshed.IsZero() {
			_, _ = fmt.Fprintf(w, "  refreshed=%s", desc.LastRefreshed.UTC().Format(time.RFC3339))
		}
		_, _ = fmt.Fprintln(w)
	}
	return nil
}

func handleClusterConnect(args []string, w io.Writer) error {
	printClusterUsage(w)
	return errors.New("cluster connect not yet implemented")
}

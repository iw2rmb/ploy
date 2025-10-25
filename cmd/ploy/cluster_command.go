package main

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

func handleCluster(args []string, w io.Writer) error {
	if len(args) == 0 {
		printClusterUsage(w)
		return errors.New("cluster subcommand required")
	}
	switch args[0] {
	case "list":
		return handleClusterList(w)
	case "add":
		return handleClusterAdd(args[1:], w)
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
		_, _ = fmt.Fprintln(w, "No clusters cached. Run 'ploy cluster add' to add one.")
		return nil
	}
	_, _ = fmt.Fprintf(w, "Clusters (%d):\n", len(descs))
	for _, desc := range descs {
		label := desc.ClusterID
		if desc.Default {
			label += " (default)"
		}
		_, _ = fmt.Fprintf(w, "  - %s  address=%s", label, desc.Address)
		if trimmed := strings.TrimSpace(desc.SSHIdentityPath); trimmed != "" {
			_, _ = fmt.Fprintf(w, "  identity=%s", trimmed)
		}
		if formatted := formatDescriptorLabels(desc.Labels); formatted != "" {
			_, _ = fmt.Fprintf(w, "  labels=%s", formatted)
		}
		_, _ = fmt.Fprintln(w)
	}
	return nil
}

func handleClusterConnect(args []string, w io.Writer) error {
	printClusterUsage(w)
	return errors.New("cluster connect not yet implemented")
}

func formatDescriptorLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(values, ",")
}

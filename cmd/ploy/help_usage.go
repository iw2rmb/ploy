package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/clitree"
)

func printModsUsage(w io.Writer) {
	printCommandUsage(w, "mods")
}

func printJobsUsage(w io.Writer) {
	printCommandUsage(w, "jobs")
}

func printClusterUsage(w io.Writer) {
	printCommandUsage(w, "cluster")
}

func printConfigUsage(w io.Writer) {
	printCommandUsage(w, "config")
}

func printCommandUsage(w io.Writer, path ...string) {
	node, ok := clitree.Lookup(path...)
	if !ok {
		fallback := fmt.Sprintf("ploy %s", strings.Join(path, " "))
		_, _ = fmt.Fprintf(w, "Usage: %s\n", fallback)
		return
	}
	renderNodeUsage(w, node, path)
}

func renderNodeUsage(w io.Writer, node clitree.Node, path []string) {
	usage := strings.TrimSpace(node.Usage)
	if usage == "" {
		usage = fmt.Sprintf("ploy %s", strings.Join(path, " "))
		if len(node.Subcommands) > 0 {
			usage += " <command>"
		}
	}
	_, _ = fmt.Fprintf(w, "Usage: %s\n", usage)

	if len(node.Subcommands) > 0 {
		_, _ = fmt.Fprintln(w, "\nCommands:")
		width := 0
		for _, child := range node.Subcommands {
			synopsis := strings.TrimSpace(child.Synopsis)
			if synopsis == "" {
				synopsis = child.Name
			}
			if l := len(synopsis); l > width {
				width = l
			}
		}
		padding := width + 2
		for _, child := range node.Subcommands {
			synopsis := strings.TrimSpace(child.Synopsis)
			if synopsis == "" {
				synopsis = child.Name
			}
			desc := strings.TrimSpace(child.Description)
			_, _ = fmt.Fprintf(w, "  %-*s %s\n", padding, synopsis, desc)
		}
	} else if desc := strings.TrimSpace(node.Description); desc != "" {
		_, _ = fmt.Fprintf(w, "\n%s\n", desc)
	}

	if note := strings.TrimSpace(node.Note); note != "" {
		_, _ = fmt.Fprintf(w, "\n%s\n", note)
	}
}

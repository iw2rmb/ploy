package main

import (
	"fmt"
	"io"
)

// printCommandUsage prints a simple usage header for the given command path.
// It intentionally mirrors the strings used by help goldens under testdata/.
func printCommandUsage(w io.Writer, parts ...string) {
	switch len(parts) {
	case 0:
		_, _ = fmt.Fprintln(w, "Usage: ploy <command>")
	case 1:
		_, _ = fmt.Fprintf(w, "Usage: ploy %s <command>\n", parts[0])
		switch parts[0] {
		case "mod":
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Commands:")
			_, _ = fmt.Fprintln(w, "  run         Submit a Mods run to the control plane")
			_, _ = fmt.Fprintln(w, "  run repo    Manage repos within a batch run (add/remove/restart/status)")
			_, _ = fmt.Fprintln(w, "  cancel      Cancel a Mods run via the control plane")
			_, _ = fmt.Fprintln(w, "  resume      Resume a paused Mods run")
			_, _ = fmt.Fprintln(w, "  inspect     Show summary for a Mods run")
			_, _ = fmt.Fprintln(w, "  artifacts   List run artifacts by stage")
			_, _ = fmt.Fprintln(w, "  diffs       List diffs or download newest patch")
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Use 'ploy mod run --help' for flag details.")
			_, _ = fmt.Fprintln(w, "Use 'ploy mod run repo' for batch repo subcommands.")
		case "server":
			// Minimal server usage
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Commands:")
			_, _ = fmt.Fprintln(w, "  deploy      Deploy and configure a control plane server")
		case "rollout":
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Commands:")
			_, _ = fmt.Fprintln(w, "  server      Roll out a new binary to a control plane server")
			_, _ = fmt.Fprintln(w, "  nodes       Roll out a new binary to worker nodes (batched)")
		case "environment":
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Commands:")
			_, _ = fmt.Fprintln(w, "  materialize Materialize integration environments from manifests")
		case "manifest":
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Commands:")
			_, _ = fmt.Fprintln(w, "  schema      Print the integration manifest JSON schema")
			_, _ = fmt.Fprintln(w, "  validate    Validate manifests and optionally rewrite them to v2")
		case "config":
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Commands:")
			_, _ = fmt.Fprintln(w, "  gitlab      Manage GitLab integration credentials")
		}
	default:
		// Print an exact usage line for deeper paths (e.g., server deploy).
		_, _ = fmt.Fprintf(w, "Usage: ploy %s\n", join(parts, " "))
	}
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += sep
		out += parts[i]
	}
	return out
}

// Minimal helpers for mods/jobs usage output paths.
func printModsUsage(w io.Writer) { _, _ = fmt.Fprintln(w, "Usage: ploy mods <command>") }
func printRunsUsage(w io.Writer) { _, _ = fmt.Fprintln(w, "Usage: ploy runs <command>") }

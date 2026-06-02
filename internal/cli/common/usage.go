package common

import (
	"fmt"
	"io"
)

// PrintCommandUsage prints a simple usage header for the given command path.
// It intentionally mirrors the strings used by help goldens under testdata/.
func PrintCommandUsage(w io.Writer, parts ...string) {
	switch len(parts) {
	case 0:
		_, _ = fmt.Fprintln(w, "Usage: ploy <command>")
	case 1:
		_, _ = fmt.Fprintf(w, "Usage: ploy %s <command>\n", parts[0])
		switch parts[0] {
		case "mig":
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Mig Project Management:")
			_, _ = fmt.Fprintln(w, "  add         Create a new mig project")
			_, _ = fmt.Fprintln(w, "  list        List mig projects")
			_, _ = fmt.Fprintln(w, "  remove      Delete a mig project")
			_, _ = fmt.Fprintln(w, "  archive     Archive a mig project")
			_, _ = fmt.Fprintln(w, "  unarchive   Unarchive a mig project")
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Spec Management:")
			_, _ = fmt.Fprintln(w, "  spec set    Set a mig's spec from a file")
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Repo Set Management:")
			_, _ = fmt.Fprintln(w, "  repo add    Add a repo to the mig")
			_, _ = fmt.Fprintln(w, "  repo list   List repos in the mig")
			_, _ = fmt.Fprintln(w, "  repo remove Remove a repo from the mig")
			_, _ = fmt.Fprintln(w, "  repo import Import repos from CSV")
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Pulling Diffs:")
			_, _ = fmt.Fprintln(w, "  pull        Pull diffs into the current git worktree")
			_, _ = fmt.Fprintln(w, "  status      Show migration status and per-run summary")
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Run Execution:")
			_, _ = fmt.Fprintln(w, "  run <mig>   Run a mig project (with optional repo selectors or --failed)")
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Artifacts:")
			_, _ = fmt.Fprintln(w, "  artifacts   List run artifacts by stage")
			_, _ = fmt.Fprintln(w, "  fetch       Download run artifacts")
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Use 'ploy mig <command> --help' for command details.")
		case "spec":
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Commands:")
			_, _ = fmt.Fprintln(w, "  schema      Print the mig JSON schema")
			_, _ = fmt.Fprintln(w, "  validate    Validate mig specs")
		case "config":
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Commands:")
			_, _ = fmt.Fprintln(w, "  env         Manage global environment variables")
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

// wantsHelp checks whether the given argument list represents a help request.
// It returns true if the sole argument is "--help" or "-h", which is the
// pattern used by command routers that manually parse arguments
// (DisableFlagParsing: true) to detect and respond to help flags before
// dispatching to subcommands.
func WantsHelp(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h")
}

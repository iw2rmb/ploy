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
			_, _ = fmt.Fprintln(w, "  run <mig>   Run a mig project (with --repo or --failed for repo selection)")
			_, _ = fmt.Fprintln(w, "  run repo    Manage repos within a batch run")
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Artifacts:")
			_, _ = fmt.Fprintln(w, "  artifacts   List run artifacts by stage")
			_, _ = fmt.Fprintln(w, "  fetch       Download run artifacts")
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Use 'ploy mig <command> --help' for command details.")
		case "manifest":
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Commands:")
			_, _ = fmt.Fprintln(w, "  schema      Print the integration manifest JSON schema")
			_, _ = fmt.Fprintln(w, "  validate    Validate manifests and optionally rewrite them to v2")
		case "config":
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Commands:")
			_, _ = fmt.Fprintln(w, "  gitlab      Manage GitLab integration credentials")
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

// Minimal helpers for run usage output paths.
// For `ploy run`, print full subcommand list.
func printRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy run <command>")
	_, _ = fmt.Fprintln(w, "       ploy run --repo <url> --base-ref <ref> --target-ref <ref> --spec <path|->")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  list        List batch runs with pagination")
	_, _ = fmt.Fprintln(w, "  status      Show status for a run (use --json for machine-readable links/artifacts)")
	_, _ = fmt.Fprintln(w, "  logs        Stream run lifecycle events (SSE)")
	_, _ = fmt.Fprintln(w, "  cancel      Cancel a run via the control plane")
	_, _ = fmt.Fprintln(w, "  start       Start pending repos for a batch run")
	_, _ = fmt.Fprintln(w, "  pull        Pull diffs into the current git worktree")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Run submission (direct flags without subcommand):")
	_, _ = fmt.Fprintln(w, "  --repo <url>       Git repository URL (https/ssh/file)")
	_, _ = fmt.Fprintln(w, "  --base-ref <ref>   Base Git ref (branch or tag)")
	_, _ = fmt.Fprintln(w, "  --target-ref <ref> Target Git ref (branch)")
	_, _ = fmt.Fprintln(w, "  --spec <path|->    Path to YAML/JSON spec file (use '-' for stdin)")
	_, _ = fmt.Fprintln(w, "  --follow           Follow run until completion")
	_, _ = fmt.Fprintln(w, "  --artifact-dir <dir> Download final artifacts after successful --follow")
	_, _ = fmt.Fprintln(w, "  --json             Print machine-readable JSON summary")
}

// wantsHelp checks whether the given argument list represents a help request.
// It returns true if the sole argument is "--help" or "-h", which is the
// pattern used by command routers that manually parse arguments
// (DisableFlagParsing: true) to detect and respond to help flags before
// dispatching to subcommands.
func wantsHelp(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h")
}

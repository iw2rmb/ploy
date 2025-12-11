package main

import (
	"errors"
	"fmt"
	"io"
)

// handleMod routes Mods subcommands to their implementations.
func handleMod(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printModUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printModUsage(stderr)
		return errors.New("mod subcommand required")
	}

	switch args[0] {
	case "run":
		return handleModRun(args[1:], stderr)
	case "fetch":
		return handleModFetch(args[1:], stderr)
	case "cancel":
		return handleModCancel(args[1:], stderr)
	case "resume":
		return handleModResume(args[1:], stderr)
	case "artifacts":
		return handleModArtifacts(args[1:], stderr)
	case "diffs":
		return handleModDiffs(args[1:], stderr)
	default:
		printModUsage(stderr)
		return fmt.Errorf("unknown mod subcommand %q", args[0])
	}
}

func printModUsage(w io.Writer) {
	printCommandUsage(w, "mod")
	// Append a concise flags summary for 'ploy mod run' to match help golden.
	// Keep this in sync with cmd/ploy/testdata/help_mod.txt.
	_, _ = fmt.Fprintln(w, "")
	printModRunFlagsSummary(w)
}

// printModRunFlagsSummary renders the 'ploy mod run' flags overview used by 'help mod'.
// This is a human-friendly summary, not the exhaustive FlagSet help.
func printModRunFlagsSummary(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Flags for 'ploy mod run':")
	_, _ = fmt.Fprintln(w, "  --spec <file>              Path to YAML/JSON spec file (CLI flags override spec values)")
	_, _ = fmt.Fprintln(w, "  --repo-url <url>           Git repository URL")
	_, _ = fmt.Fprintln(w, "  --repo-base-ref <branch>   Git base ref")
	_, _ = fmt.Fprintln(w, "  --repo-target-ref <branch> Git target ref")
	_, _ = fmt.Fprintln(w, "  --repo-workspace-hint <dir> Optional subdirectory hint")
	_, _ = fmt.Fprintln(w, "  --mod-env KEY=VALUE        Mod environment (repeatable)")
	_, _ = fmt.Fprintln(w, "  --mod-image <image>        Container image for mod step")
	_, _ = fmt.Fprintln(w, "  --mod-command <cmd>        Container command override")
	_, _ = fmt.Fprintln(w, "  --retain-container         Retain container after execution")
	_, _ = fmt.Fprintln(w, "  --gitlab-pat <token>       GitLab Personal Access Token")
	_, _ = fmt.Fprintln(w, "  --gitlab-domain <domain>   GitLab domain")
	_, _ = fmt.Fprintln(w, "  --mr-success               Create merge request on success")
	_, _ = fmt.Fprintln(w, "  --mr-fail                  Create merge request on failure")
	_, _ = fmt.Fprintln(w, "  --follow                   Follow run events until completion")
	_, _ = fmt.Fprintln(w, "  --cap <duration>           Time cap for --follow")
	_, _ = fmt.Fprintln(w, "  --cancel-on-cap            Cancel run if cap exceeded")
	_, _ = fmt.Fprintln(w, "  --artifact-dir <dir>       Download final artifacts")
	_, _ = fmt.Fprintln(w, "  --json                     Print JSON summary")
	_, _ = fmt.Fprintln(w, "  --max-retries N            Max reconnect attempts for event stream")
}

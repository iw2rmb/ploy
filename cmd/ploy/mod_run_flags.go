// mod_run_flags.go separates CLI flag definitions from execution logic.
//
// This file defines flag parsing for mod run command including --follow,
// --cap, --download, --spec, and various manifest options. Flag definitions
// are isolated from execution to enable reusability and focused testing of
// flag precedence and validation without coupling to HTTP submission flow.
package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

// stringSlice is a simple flag.Value for collecting repeated values.
type stringSlice []string

func (s *stringSlice) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// modRunFlags encapsulates all CLI flags for the mod run command.
// This struct provides a clean separation between flag definitions and execution logic.
type modRunFlags struct {
	// Spec file path
	SpecFile *string

	// Repository configuration
	RepoURL           *string
	RepoBaseRef       *string
	RepoTargetRef     *string
	RepoWorkspaceHint *string

	// Follow/polling behavior
	Follow      *bool
	CapDuration *time.Duration
	CancelOnCap *bool
	MaxRetries  *int
	RetryWait   *time.Duration
	LogFormat   *string // Log output format: "structured" or "raw"

	// Artifact and output
	ArtifactDir *string
	JSONOut     *bool

	// Mod container configuration
	ModEnvs    *stringSlice
	ModImage   *string
	ModCommand *string
	Retain     *bool

	// GitLab integration (per-run overrides)
	GitLabPAT    *string
	GitLabDomain *string
	MRSuccess    *bool
	MRFail       *bool

	// Deprecated flags
	HealOnBuild *bool
}

// parseModRunFlags creates a FlagSet, defines all mod run flags, and parses the provided arguments.
// Returns the parsed flags or an error if parsing fails.
func parseModRunFlags(args []string) (*modRunFlags, error) {
	fs := flag.NewFlagSet("mod run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	flags := &modRunFlags{}

	// Spec file configuration
	flags.SpecFile = fs.String("spec", "", "Path to YAML/JSON spec file")

	// Repository configuration
	flags.RepoURL = fs.String("repo-url", "", "Git repository URL to materialise for Mods execution")
	flags.RepoBaseRef = fs.String("repo-base-ref", "", "Git base ref used for materialisation")
	flags.RepoTargetRef = fs.String("repo-target-ref", "", "Git target ref created for the run")
	flags.RepoWorkspaceHint = fs.String("repo-workspace-hint", "", "Optional subdirectory hint when preparing the workspace")

	// Follow/polling behavior
	flags.Follow = fs.Bool("follow", false, "follow ticket events until completion")
	flags.CapDuration = fs.Duration("cap", 0, "optional overall time cap for --follow (e.g., 5m)")
	flags.CancelOnCap = fs.Bool("cancel-on-cap", false, "when set with --cap, cancel the ticket if the cap is exceeded")
	flags.MaxRetries = fs.Int("max-retries", 5, "max reconnect attempts for event stream (-1 for unlimited)")
	flags.RetryWait = fs.Duration("retry-wait", 500*time.Millisecond, "wait between event stream reconnects")
	flags.LogFormat = fs.String("log-format", "structured", "log output format: structured (default, with context) or raw (message only)")

	// Artifact and output
	flags.ArtifactDir = fs.String("artifact-dir", "", "directory to download final artifacts into (with manifest.json)")
	flags.JSONOut = fs.Bool("json", false, "print machine-readable JSON summary to stdout")

	// Mod container configuration
	flags.ModEnvs = new(stringSlice)
	fs.Var(flags.ModEnvs, "mod-env", "Mod environment KEY=VALUE (repeatable)")
	flags.ModImage = fs.String("mod-image", "", "Container image for the mod step (optional)")
	flags.ModCommand = fs.String("mod-command", "", "Container command override (string or JSON array)")
	flags.Retain = fs.Bool("retain-container", false, "Retain the mod container after execution (for debugging)")

	// GitLab integration (per-run overrides)
	flags.GitLabPAT = fs.String("gitlab-pat", "", "GitLab Personal Access Token for this run (overrides server default)")
	flags.GitLabDomain = fs.String("gitlab-domain", "", "GitLab domain for this run (overrides server default)")
	flags.MRSuccess = fs.Bool("mr-success", false, "Create a merge request on success")
	flags.MRFail = fs.Bool("mr-fail", false, "Create a merge request on failure")

	// Deprecated flags
	flags.HealOnBuild = fs.Bool("heal-on-build", false, "DEPRECATED: inject default build_gate_healing (use --spec with build_gate_healing instead)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	return flags, nil
}

// printModRunUsage writes usage information for the mod run command to the provided writer.
func printModRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run [--spec <file>] [--repo-url <url> --repo-base-ref <branch> --repo-target-ref <branch> --repo-workspace-hint <dir>] [--mod-env KEY=VALUE ...] [--mod-image <image>] [--mod-command <cmd>] [--retain-container] [--gitlab-pat <token>] [--gitlab-domain <domain>] [--mr-success] [--mr-fail] [--heal-on-build (deprecated)] [--follow] [--log-format structured|raw] [--cap <duration>] [--artifact-dir <dir>] [--json] [--max-retries N] [--retry-wait D]")
}

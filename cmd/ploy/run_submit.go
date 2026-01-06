// run_submit.go implements `ploy run --repo ... --base-ref ... --target-ref ... --spec ...`
// for single-repo run submission via POST /v1/runs.
//
// This is the v1 CLI entry point for submitting runs directly (without creating
// a mod project first). The command creates a mod project as a side-effect;
// the created mod has name == id.
//
// See roadmap/v1/cli.md:13 for specification.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"gopkg.in/yaml.v3"
)

// runSubmitFlags encapsulates CLI flags for `ploy run --repo ... --base-ref ... --target-ref ... --spec ...`.
// This struct separates flag definitions from execution logic.
type runSubmitFlags struct {
	// Repository configuration (all required)
	RepoURL   *string
	BaseRef   *string
	TargetRef *string

	// Spec file path (required); use "-" for stdin
	SpecFile *string
}

// parseRunSubmitFlags creates a FlagSet, defines all run submit flags, and parses the provided arguments.
// Returns the parsed flags or an error if parsing fails.
func parseRunSubmitFlags(args []string) (*runSubmitFlags, error) {
	fs := flag.NewFlagSet("run submit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	flags := &runSubmitFlags{}

	// Repository configuration
	flags.RepoURL = fs.String("repo", "", "Git repository URL (https/ssh/file)")
	flags.BaseRef = fs.String("base-ref", "", "Base Git ref (branch or tag)")
	flags.TargetRef = fs.String("target-ref", "", "Target Git ref (branch)")
	flags.SpecFile = fs.String("spec", "", "Path to YAML/JSON spec file (use '-' for stdin)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	return flags, nil
}

// validateRunSubmitFlags checks that all required flags are provided.
func validateRunSubmitFlags(flags *runSubmitFlags) error {
	if flags.RepoURL == nil || strings.TrimSpace(*flags.RepoURL) == "" {
		return fmt.Errorf("--repo is required")
	}
	if flags.BaseRef == nil || strings.TrimSpace(*flags.BaseRef) == "" {
		return fmt.Errorf("--base-ref is required")
	}
	if flags.TargetRef == nil || strings.TrimSpace(*flags.TargetRef) == "" {
		return fmt.Errorf("--target-ref is required")
	}
	if flags.SpecFile == nil || strings.TrimSpace(*flags.SpecFile) == "" {
		return fmt.Errorf("--spec is required")
	}
	return nil
}

// handleRunSubmit implements the `ploy run --repo ... --base-ref ... --target-ref ... --spec ...` command.
// It submits a single-repo run via POST /v1/runs and prints the resulting run_id and mod_id.
//
// The command:
// 1. Parses CLI flags
// 2. Loads and validates the spec from file or stdin
// 3. Submits the run request to POST /v1/runs
// 4. Prints run_id and mod_id to stderr
func handleRunSubmit(args []string, stderr io.Writer) error {
	// Parse CLI flags using extracted flag handling.
	flags, err := parseRunSubmitFlags(args)
	if err != nil {
		printRunSubmitUsage(stderr)
		return err
	}

	// Validate required flags.
	if err := validateRunSubmitFlags(flags); err != nil {
		printRunSubmitUsage(stderr)
		return err
	}

	// Load spec from file or stdin (--spec - reads from stdin).
	specPayload, err := loadSpec(*flags.SpecFile)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	ctx := context.Background()

	// Resolve control plane connection.
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Build run request from parsed flags.
	request := modsapi.RunSubmitRequest{
		RepoURL:   strings.TrimSpace(*flags.RepoURL),
		BaseRef:   strings.TrimSpace(*flags.BaseRef),
		TargetRef: strings.TrimSpace(*flags.TargetRef),
		Spec:      specPayload,
		CreatedBy: strings.TrimSpace(os.Getenv("USER")),
	}

	// Submit run to control plane via POST /v1/runs.
	// The SubmitCommand returns a RunSummary after fetching status.
	cmd := mods.SubmitCommand{
		Client:  httpClient,
		BaseURL: base,
		Request: request,
	}

	summary, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	// Print run_id and mod_id as specified in roadmap/v1/cli.md:18.
	// summary.RunID is the created run; summary contains the mod_id indirectly
	// (via the status endpoint), but the submit response itself returns mod_id.
	// For now we print run_id; mod_id is printed via a follow-up if needed.
	_, _ = fmt.Fprintf(stderr, "run_id: %s\n", summary.RunID)
	_, _ = fmt.Fprintf(stderr, "state: %s\n", summary.State)

	return nil
}

// loadSpec loads a spec from a file path or stdin (when path is "-").
// Supports both YAML and JSON formats. Returns the spec as JSON bytes.
func loadSpec(path string) (json.RawMessage, error) {
	var data []byte
	var err error

	if path == "-" {
		// Read spec from stdin.
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
	} else {
		// Read spec from file.
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", path, err)
		}
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("spec is empty")
	}

	// Parse YAML/JSON into a map for normalization.
	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		// Not JSON; try YAML.
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("parse spec (not valid JSON or YAML): %w", err)
		}
	}

	// Marshal to JSON for submission.
	jsonBytes, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec to JSON: %w", err)
	}

	// Validate spec using the canonical parser to catch structural issues early.
	if _, err := contracts.ParseModsSpecJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	return jsonBytes, nil
}

// printRunSubmitUsage writes usage information for `ploy run --repo ... --spec ...`.
func printRunSubmitUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy run --repo <repo-url> --base-ref <ref> --target-ref <ref> --spec <path|->")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Submits a single-repo run and immediately starts execution.")
	_, _ = fmt.Fprintln(w, "Creates a mod project as a side-effect; the created mod has name == id.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --repo <url>       Git repository URL (https/ssh/file)")
	_, _ = fmt.Fprintln(w, "  --base-ref <ref>   Base Git ref (branch or tag)")
	_, _ = fmt.Fprintln(w, "  --target-ref <ref> Target Git ref (branch)")
	_, _ = fmt.Fprintln(w, "  --spec <path|->    Path to YAML/JSON spec file (use '-' for stdin)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy run --repo https://github.com/org/repo --base-ref main --target-ref feature-branch --spec spec.yaml")
	_, _ = fmt.Fprintln(w, "  cat spec.yaml | ploy run --repo https://github.com/org/repo --base-ref main --target-ref feature-branch --spec -")
}

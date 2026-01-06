// run_submit.go implements `ploy run --repo ... --base-ref ... --target-ref ... --spec ...`
// for single-repo run submission via POST /v1/runs.
//
// This is the CLI entry point for submitting runs directly (without creating
// a mod project first). The command creates a mod project as a side-effect;
// the created mod has name == id.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

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

	runID, modID, err := submitSingleRepoRun(ctx, base, httpClient, request)
	if err != nil {
		return err
	}

	// Print run_id and mod_id on success.
	_, _ = fmt.Fprintf(stderr, "run_id: %s\n", runID)
	_, _ = fmt.Fprintf(stderr, "mod_id: %s\n", modID)

	return nil
}

func submitSingleRepoRun(ctx context.Context, base *url.URL, httpClient *http.Client, request modsapi.RunSubmitRequest) (runID string, modID string, err error) {
	if base == nil {
		return "", "", fmt.Errorf("run submit: base url required")
	}
	if httpClient == nil {
		return "", "", fmt.Errorf("run submit: http client required")
	}

	endpoint := base.ResolveReference(&url.URL{Path: "/v1/runs"})
	payload, err := json.Marshal(request)
	if err != nil {
		return "", "", fmt.Errorf("run submit: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("run submit: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("run submit: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &apiErr); err == nil {
			if msg := strings.TrimSpace(apiErr.Error); msg != "" {
				return "", "", fmt.Errorf("run submit: %s", msg)
			}
		}
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return "", "", fmt.Errorf("run submit: %s", msg)
	}

	var created struct {
		RunID  string `json:"run_id"`
		ModID  string `json:"mod_id"`
		SpecID string `json:"spec_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", "", fmt.Errorf("run submit: decode response: %w", err)
	}
	created.RunID = strings.TrimSpace(created.RunID)
	created.ModID = strings.TrimSpace(created.ModID)
	if created.RunID == "" {
		return "", "", fmt.Errorf("run submit: empty run_id in response")
	}
	if created.ModID == "" {
		return "", "", fmt.Errorf("run submit: empty mod_id in response")
	}
	return created.RunID, created.ModID, nil
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

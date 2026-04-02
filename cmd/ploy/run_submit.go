// run_submit.go implements `ploy run --repo ... --base-ref ... --target-ref ... --spec ...`
// for single-repo run submission via POST /v1/runs.
//
// This is the CLI entry point for submitting runs directly (without creating
// a mig project first). The command creates a mig project as a side-effect;
// the created mig has name == id.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/runs"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
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

	// Follow mode flags
	Follow      *bool
	CapDuration *time.Duration
	CancelOnCap *bool
	MaxRetries  *int

	// Job container configuration
	MigEnvs    *stringSlice
	JobImage   *string
	MigCommand *string

	// GitLab integration
	GitLabPAT    *string
	GitLabDomain *string
	MRSuccess    *bool
	MRFail       *bool

	// Artifact and output
	ArtifactDir *string
	JSONOut     *bool
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

	// Follow mode flags
	flags.Follow = fs.Bool("follow", false, "Follow run until completion (shows job graph)")
	flags.CapDuration = new(time.Duration)
	fs.DurationVar(flags.CapDuration, "cap", 0, "Optional time cap for --follow")
	flags.CancelOnCap = fs.Bool("cancel-on-cap", false, "Cancel run if cap exceeded")
	flags.MaxRetries = fs.Int("max-retries", 5, "Max report fetch retries (-1 for unlimited)")

	// Job container configuration
	flags.MigEnvs = new(stringSlice)
	fs.Var(flags.MigEnvs, "job-env", "Job environment KEY=VALUE (repeatable)")
	flags.JobImage = fs.String("job-image", "", "Container image for the mig step (optional)")
	flags.MigCommand = fs.String("job-command", "", "Container command override")

	// GitLab integration
	flags.GitLabPAT = fs.String("gitlab-pat", "", "GitLab Personal Access Token for this run (overrides server default)")
	flags.GitLabDomain = fs.String("gitlab-domain", "", "GitLab domain for this run (overrides server default)")
	flags.MRSuccess = fs.Bool("mr-success", false, "Create a merge request on success")
	flags.MRFail = fs.Bool("mr-fail", false, "Create a merge request on failure")

	// Artifact and output
	flags.ArtifactDir = fs.String("artifact-dir", "", "directory to download final artifacts into (with manifest.json)")
	flags.JSONOut = fs.Bool("json", false, "print machine-readable JSON summary to stdout")

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
// It submits a single-repo run via POST /v1/runs and prints the resulting run_id and mig_id.
//
// The command:
// 1. Parses CLI flags
// 2. Loads and validates the spec from file or stdin
// 3. Submits the run request to POST /v1/runs
// 4. Prints run_id and mig_id to stderr
func handleRunSubmit(args []string, stderr io.Writer) error {
	// Parse CLI flags using extracted flag handling.
	flags, err := parseRunSubmitFlags(args)
	if err != nil {
		printRunSubmitUsage(stderr)
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	// Validate required flags.
	if err := validateRunSubmitFlags(flags); err != nil {
		printRunSubmitUsage(stderr)
		return err
	}

	ctx := context.Background()

	// Resolve control plane connection (needed before spec processing for bundle upload).
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Load spec from file or stdin and apply CLI overrides.
	specPayload, err := buildRunSubmitSpecPayload(ctx, base, httpClient, flags)
	if err != nil {
		return err
	}

	// Build run request from parsed flags.
	request := domainapi.RunSubmitRequest{
		RepoURL:   domaintypes.RepoURL(strings.TrimSpace(*flags.RepoURL)),
		BaseRef:   domaintypes.GitRef(strings.TrimSpace(*flags.BaseRef)),
		TargetRef: domaintypes.GitRef(strings.TrimSpace(*flags.TargetRef)),
		Spec:      specPayload,
		CreatedBy: strings.TrimSpace(os.Getenv("USER")),
	}

	runID, migID, err := submitSingleRepoRun(ctx, base, httpClient, request)
	if err != nil {
		return err
	}

	followEnabled := flags.Follow != nil && *flags.Follow
	// Print run_id and mig_id only in non-follow mode; follow output already includes Run/Mig headers.
	if !followEnabled {
		_, _ = fmt.Fprintf(stderr, "run_id: %s\n", runID.String())
		_, _ = fmt.Fprintf(stderr, "mig_id: %s\n", migID.String())
	}

	initialState := "pending"
	finalState := ""

	// Follow mode: display job graph until completion.
	if followEnabled {
		final, err := followRunSubmit(ctx, base, httpClient, runID, flags, stderr)
		if err != nil {
			return err
		}
		finalState = strings.ToLower(string(final))

		// Download artifacts after successful completion.
		if final == migsapi.RunStateSucceeded {
			if artifactDir := strings.TrimSpace(*flags.ArtifactDir); artifactDir != "" {
				if err := downloadRunArtifacts(ctx, base, httpClient, runID.String(), artifactDir, stderr); err != nil {
					return err
				}
			}
		}
	}

	if *flags.JSONOut {
		if err := outputRunSubmitJSONSummary(ctx, base, httpClient, runID, initialState, finalState, flags); err != nil {
			return err
		}
	}

	return nil
}

func buildRunSubmitSpecPayload(ctx context.Context, base *url.URL, client *http.Client, flags *runSubmitFlags) (json.RawMessage, error) {
	specPath := strings.TrimSpace(*flags.SpecFile)
	if specPath == "" {
		return nil, fmt.Errorf("load spec: spec path is empty")
	}

	if specPath == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("load spec: read stdin: %w", err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("load spec: spec is empty")
		}
		f, err := os.CreateTemp("", "ploy-run-spec-*.yaml")
		if err != nil {
			return nil, fmt.Errorf("load spec: create temp file: %w", err)
		}
		tempPath := f.Name()
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			_ = os.Remove(tempPath)
			return nil, fmt.Errorf("load spec: write temp file: %w", err)
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(tempPath)
			return nil, fmt.Errorf("load spec: close temp file: %w", err)
		}
		defer func() { _ = os.Remove(tempPath) }()
		specPath = tempPath
	}

	migEnvs := []string{}
	if flags.MigEnvs != nil {
		migEnvs = append(migEnvs, (*flags.MigEnvs)...)
	}
	migImage := ""
	if flags.JobImage != nil {
		migImage = strings.TrimSpace(*flags.JobImage)
	}
	migCommand := ""
	if flags.MigCommand != nil {
		migCommand = strings.TrimSpace(*flags.MigCommand)
	}
	gitlabPAT := ""
	if flags.GitLabPAT != nil {
		gitlabPAT = strings.TrimSpace(*flags.GitLabPAT)
	}
	gitlabDomain := ""
	if flags.GitLabDomain != nil {
		gitlabDomain = strings.TrimSpace(*flags.GitLabDomain)
	}
	mrSuccess := false
	if flags.MRSuccess != nil {
		mrSuccess = *flags.MRSuccess
	}
	mrFail := false
	if flags.MRFail != nil {
		mrFail = *flags.MRFail
	}

	specPayload, err := buildSpecPayload(
		ctx,
		base,
		client,
		specPath,
		migEnvs,
		migImage,
		false,
		migCommand,
		gitlabPAT,
		gitlabDomain,
		mrSuccess,
		mrFail,
	)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	if len(specPayload) == 0 {
		return nil, fmt.Errorf("load spec: spec is empty")
	}
	return specPayload, nil
}

// followRunSubmit displays run status frames until run completion.
func followRunSubmit(ctx context.Context, baseURL *url.URL, client *http.Client, runID domaintypes.RunID, flags *runSubmitFlags, stderr io.Writer) (migsapi.RunState, error) {
	followCtx := ctx
	var cancel context.CancelFunc
	capDuration := time.Duration(0)
	if flags.CapDuration != nil {
		capDuration = *flags.CapDuration
	}
	if capDuration > 0 {
		followCtx, cancel = context.WithTimeout(ctx, capDuration)
		defer cancel()
	}

	token, err := resolveControlPlaneToken()
	if err != nil {
		return "", err
	}

	renderOpts := runs.TextRenderOptions{
		EnableOSC8:    runStatusSupportsOSC8(stderr),
		AuthToken:     token,
		BaseURL:       baseURL,
		LiveDurations: true,
	}

	maxRetries := 5
	if flags.MaxRetries != nil {
		maxRetries = *flags.MaxRetries
	}
	final, err := runs.FollowRunCommand{
		Client:       client,
		BaseURL:      baseURL,
		RunID:        runID,
		Output:       stderr,
		EnableOSC8:   renderOpts.EnableOSC8,
		AuthToken:    renderOpts.AuthToken,
		MaxRetries:   maxRetries,
		PollInterval: time.Second,
	}.Run(followCtx)
	if err != nil {
		cancelOnCap := flags.CancelOnCap != nil && *flags.CancelOnCap
		if capDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
			if cancelOnCap {
				_, _ = fmt.Fprintln(stderr, "Follow timed out; requesting run cancellation...")
				_ = runs.CancelCommand{
					BaseURL: baseURL,
					Client:  client,
					RunID:   runID,
					Reason:  "cap exceeded",
					Output:  stderr,
				}.Run(context.Background())
			} else {
				_, _ = fmt.Fprintf(stderr, "Follow capped after %s; run %s continues running in the background.\n", capDuration.String(), runID)
			}
			return "", nil
		}
		return "", err
	}

	if final == "" {
		if capDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
			cancelOnCap := flags.CancelOnCap != nil && *flags.CancelOnCap
			if cancelOnCap {
				_, _ = fmt.Fprintln(stderr, "Follow timed out; requesting run cancellation...")
				_ = runs.CancelCommand{
					BaseURL: baseURL,
					Client:  client,
					RunID:   runID,
					Reason:  "cap exceeded",
					Output:  stderr,
				}.Run(context.Background())
			} else {
				_, _ = fmt.Fprintf(stderr, "Follow capped after %s; run %s continues running in the background.\n", capDuration.String(), runID)
			}
			return "", nil
		}
		return "", nil
	}

	_, _ = fmt.Fprintf(stderr, "Final state: %s\n", strings.ToLower(string(final)))
	if final != migsapi.RunStateSucceeded {
		return final, fmt.Errorf("run ended in %s", strings.ToLower(string(final)))
	}
	return final, nil
}

func submitSingleRepoRun(ctx context.Context, base *url.URL, httpClient *http.Client, request domainapi.RunSubmitRequest) (domaintypes.RunID, domaintypes.MigID, error) {
	if base == nil {
		return "", "", fmt.Errorf("run submit: base url required")
	}
	if httpClient == nil {
		return "", "", fmt.Errorf("run submit: http client required")
	}

	endpoint := base.JoinPath("v1", "runs")
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
		RunID  domaintypes.RunID  `json:"run_id"`
		MigID  domaintypes.MigID  `json:"mig_id"`
		SpecID domaintypes.SpecID `json:"spec_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", "", fmt.Errorf("run submit: decode response: %w", err)
	}
	if created.RunID.IsZero() {
		return "", "", fmt.Errorf("run submit: empty run_id in response")
	}
	if created.MigID.IsZero() {
		return "", "", fmt.Errorf("run submit: empty mig_id in response")
	}
	return created.RunID, created.MigID, nil
}

// loadSpec loads a spec from a file path or stdin (when path is "-").
// Supports both YAML and JSON formats. Returns the spec as JSON bytes.
func loadSpec(ctx context.Context, base *url.URL, client *http.Client, path string) (json.RawMessage, error) {
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

	// Parse YAML/JSON, run shared CLI preprocessing (spec_path/env_from_file/tmp_bundle),
	// then validate with the canonical parser to catch structural issues early.
	return normalizeMigsSpecToJSON(ctx, base, client, data)
}

func outputRunSubmitJSONSummary(ctx context.Context, base *url.URL, httpClient *http.Client, runID domaintypes.RunID, initialState, finalState string, flags *runSubmitFlags) error {
	// Optional: probe MR URL from run status metadata.
	mrURL, _ := fetchMRURL(ctx, base, httpClient, runID.String())

	type runJSON struct {
		RunID       domaintypes.RunID `json:"run_id"`
		Initial     string            `json:"initial_state,omitempty"`
		Final       string            `json:"final_state,omitempty"`
		ArtifactDir string            `json:"artifact_dir,omitempty"`
		MRURL       string            `json:"mr_url,omitempty"`
	}

	out := runJSON{
		RunID:   runID,
		Initial: initialState,
		Final:   finalState,
	}

	if s := strings.TrimSpace(*flags.ArtifactDir); s != "" {
		out.ArtifactDir = s
	}
	if mrURL != "" {
		out.MRURL = mrURL
	}

	b, _ := json.Marshal(out)
	fmt.Println(string(b))
	return nil
}

// printRunSubmitUsage writes usage information for `ploy run --repo ... --spec ...`.
func printRunSubmitUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy run --repo <repo-url> --base-ref <ref> --target-ref <ref> --spec <path|-> [--follow]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Submits a single-repo run and immediately starts execution.")
	_, _ = fmt.Fprintln(w, "Creates a mig project as a side-effect; the created mig has name == id.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Required flags:")
	_, _ = fmt.Fprintln(w, "  --repo <url>       Git repository URL (https/ssh/file)")
	_, _ = fmt.Fprintln(w, "  --base-ref <ref>   Base Git ref (branch or tag)")
	_, _ = fmt.Fprintln(w, "  --target-ref <ref> Target Git ref (branch)")
	_, _ = fmt.Fprintln(w, "  --spec <path|->    Path to YAML/JSON spec file (use '-' for stdin)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Follow mode:")
	_, _ = fmt.Fprintln(w, "  --follow            Follow run until completion (shows job graph)")
	_, _ = fmt.Fprintln(w, "  --cap <duration>    Optional time cap for --follow (e.g., 30m, 1h)")
	_, _ = fmt.Fprintln(w, "  --cancel-on-cap     Cancel run if cap exceeded")
	_, _ = fmt.Fprintln(w, "  --max-retries <n>   Max report fetch retries (-1 for unlimited)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Spec overrides:")
	_, _ = fmt.Fprintln(w, "  --job-env KEY=VALUE  Job environment (repeatable)")
	_, _ = fmt.Fprintln(w, "  --job-image <image>  Container image for mig step")
	_, _ = fmt.Fprintln(w, "  --job-command <cmd>  Container command override")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "GitLab options:")
	_, _ = fmt.Fprintln(w, "  --gitlab-pat <token>      GitLab Personal Access Token")
	_, _ = fmt.Fprintln(w, "  --gitlab-domain <domain>  GitLab domain")
	_, _ = fmt.Fprintln(w, "  --mr-success              Create merge request on success")
	_, _ = fmt.Fprintln(w, "  --mr-fail                 Create merge request on failure")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Artifacts/output:")
	_, _ = fmt.Fprintln(w, "  --artifact-dir <dir>  Download final artifacts after successful --follow")
	_, _ = fmt.Fprintln(w, "  --json                Print machine-readable JSON summary to stdout")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy run --repo https://github.com/org/repo --base-ref main --target-ref feature-branch --spec spec.yaml")
	_, _ = fmt.Fprintln(w, "  ploy run --repo https://github.com/org/repo --base-ref main --target-ref feature-branch --spec spec.yaml --follow")
	_, _ = fmt.Fprintln(w, "  cat spec.yaml | ploy run --repo https://github.com/org/repo --base-ref main --target-ref feature-branch --spec -")
}

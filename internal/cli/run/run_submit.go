// run_submit.go implements `ploy run --repo ... --base-ref ... --target-ref ... --spec ...`
// for single-repo run submission via POST /v1/runs.
//
// This is the CLI entry point for submitting runs directly (without creating
// a mig project first). The command creates a mig project as a side-effect;
// the created mig has name == id.
package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	"github.com/iw2rmb/ploy/internal/cli/specpayload"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// SubmitOptions contains Cobra-parsed options for `ploy run --repo ...`.
type SubmitOptions struct {
	RepoURL   string
	BaseRef   string
	TargetRef string
	SpecFile  string

	Follow      bool
	CapDuration time.Duration
	CancelOnCap bool
	MaxRetries  int

	MigEnvs    []string
	JobImage   string
	MigCommand string

	ArtifactDir  string
	JSONOut      bool
	Output       io.Writer
	FollowOutput io.Writer
}

// validateRunSubmitFlags checks that all required flags are provided.
func validateRunSubmitFlags(opts SubmitOptions) error {
	if strings.TrimSpace(opts.RepoURL) == "" {
		return fmt.Errorf("--repo is required")
	}
	if strings.TrimSpace(opts.BaseRef) == "" {
		return fmt.Errorf("--base-ref is required")
	}
	if strings.TrimSpace(opts.TargetRef) == "" {
		return fmt.Errorf("--target-ref is required")
	}
	if strings.TrimSpace(opts.SpecFile) == "" {
		return fmt.Errorf("--spec is required")
	}
	return nil
}

// RunSubmit implements the `ploy run --repo ... --base-ref ... --target-ref ... --spec ...` command.
// It submits a single-repo run via POST /v1/runs and prints the resulting run_id and mig_id.
func RunSubmit(ctx context.Context, opts SubmitOptions) error {
	if opts.Output == nil {
		opts.Output = io.Discard
	}
	if opts.FollowOutput == nil {
		opts.FollowOutput = opts.Output
	}
	if err := validateRunSubmitFlags(opts); err != nil {
		return err
	}

	// Resolve control plane connection (needed before spec processing for bundle upload).
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Load spec from file or stdin and apply CLI overrides.
	specPayload, err := buildRunSubmitSpecPayload(ctx, base, httpClient, opts)
	if err != nil {
		return err
	}

	// Build run request from parsed flags.
	request := domainapi.RunSubmitRequest{
		RepoURL:   domaintypes.RepoURL(strings.TrimSpace(opts.RepoURL)),
		BaseRef:   domaintypes.GitRef(strings.TrimSpace(opts.BaseRef)),
		TargetRef: domaintypes.GitRef(strings.TrimSpace(opts.TargetRef)),
		Spec:      specPayload,
		CreatedBy: strings.TrimSpace(os.Getenv("USER")),
	}

	runID, migID, err := submitSingleRepoRun(ctx, base, httpClient, request)
	if err != nil {
		return err
	}

	// Print run_id and mig_id only in non-follow mode; follow output already includes Run/Mig headers.
	if !opts.Follow && !opts.JSONOut {
		_, _ = fmt.Fprintf(opts.Output, "run_id: %s\n", runID.String())
		_, _ = fmt.Fprintf(opts.Output, "mig_id: %s\n", migID.String())
	}

	initialState := "pending"
	finalState := ""

	// Follow mode: display job graph until completion.
	if opts.Follow {
		final, err := followRunSubmit(ctx, base, httpClient, runID, opts)
		if err != nil {
			return err
		}
		finalState = strings.ToLower(string(final))

		// Download artifacts after successful completion.
		if final == migsapi.RunStateSucceeded {
			if artifactDir := strings.TrimSpace(opts.ArtifactDir); artifactDir != "" {
				if err := DownloadRunArtifacts(ctx, base, httpClient, runID.String(), artifactDir, opts.FollowOutput); err != nil {
					return err
				}
			}
		}
	}

	if opts.JSONOut {
		if err := outputRunSubmitJSONSummary(opts.Output, runID, initialState, finalState, opts); err != nil {
			return err
		}
	}

	return nil
}

func buildRunSubmitSpecPayload(ctx context.Context, base *url.URL, client *http.Client, opts SubmitOptions) (json.RawMessage, error) {
	specPath := strings.TrimSpace(opts.SpecFile)
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
		tempDir, wdErr := os.Getwd()
		if wdErr != nil {
			tempDir = ""
		}
		f, err := os.CreateTemp(tempDir, "ploy-run-spec-*.yaml")
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
	migEnvs = append(migEnvs, opts.MigEnvs...)
	migImage := strings.TrimSpace(opts.JobImage)
	migCommand := strings.TrimSpace(opts.MigCommand)
	specPayload, err := specpayload.Build(
		ctx,
		base,
		client,
		specPath,
		migEnvs,
		migImage,
		false,
		migCommand,
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
func followRunSubmit(ctx context.Context, baseURL *url.URL, client *http.Client, runID domaintypes.RunID, opts SubmitOptions) (migsapi.RunState, error) {
	followCtx := ctx
	var cancel context.CancelFunc
	capDuration := opts.CapDuration
	if capDuration > 0 {
		followCtx, cancel = context.WithTimeout(ctx, capDuration)
		defer cancel()
	}

	renderOpts := common.FollowRunRenderOptions(baseURL, opts.FollowOutput)

	maxRetries := 5
	if opts.MaxRetries != 0 {
		maxRetries = opts.MaxRetries
	}
	final, err := runs.FollowRunCommand{
		Client:       client,
		BaseURL:      baseURL,
		RunID:        runID,
		Output:       opts.FollowOutput,
		EnableOSC8:   renderOpts.EnableOSC8,
		AuthToken:    renderOpts.AuthToken,
		MaxRetries:   maxRetries,
		PollInterval: time.Second,
	}.Run(followCtx)
	if err != nil {
		if capDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
			if opts.CancelOnCap {
				_, _ = fmt.Fprintln(opts.FollowOutput, "Follow timed out; requesting run cancellation...")
				_ = runs.CancelCommand{
					BaseURL: baseURL,
					Client:  client,
					RunID:   runID,
					Reason:  "cap exceeded",
					Output:  opts.FollowOutput,
				}.Run(context.Background())
			} else {
				_, _ = fmt.Fprintf(opts.FollowOutput, "Follow capped after %s; run %s continues running in the background.\n", capDuration.String(), runID)
			}
			return "", nil
		}
		return "", err
	}

	if final == "" {
		if capDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
			if opts.CancelOnCap {
				_, _ = fmt.Fprintln(opts.FollowOutput, "Follow timed out; requesting run cancellation...")
				_ = runs.CancelCommand{
					BaseURL: baseURL,
					Client:  client,
					RunID:   runID,
					Reason:  "cap exceeded",
					Output:  opts.FollowOutput,
				}.Run(context.Background())
			} else {
				_, _ = fmt.Fprintf(opts.FollowOutput, "Follow capped after %s; run %s continues running in the background.\n", capDuration.String(), runID)
			}
			return "", nil
		}
		return "", nil
	}

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

func outputRunSubmitJSONSummary(w io.Writer, runID domaintypes.RunID, initialState, finalState string, opts SubmitOptions) error {
	type runJSON struct {
		RunID       domaintypes.RunID `json:"run_id"`
		Initial     string            `json:"initial_state,omitempty"`
		Final       string            `json:"final_state,omitempty"`
		ArtifactDir string            `json:"artifact_dir,omitempty"`
	}

	payload := runJSON{
		RunID:   runID,
		Initial: initialState,
		Final:   finalState,
	}

	if s := strings.TrimSpace(opts.ArtifactDir); s != "" {
		payload.ArtifactDir = s
	}
	b, _ := json.Marshal(payload)
	_, err := fmt.Fprintln(w, string(b))
	return err
}

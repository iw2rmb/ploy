package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/httpx"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	"github.com/iw2rmb/ploy/internal/cli/specpayload"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

const osTempArtifactDirSentinel = "__ploy_os_tmp__"

// SubmitOptions contains Cobra-parsed options for `ploy run <spec-path> [repo]`.
type SubmitOptions struct {
	SpecPath     string
	RepoSelector string
	Follow       bool
	Apply        bool

	PullArtifacts bool
	PullPath      string

	MaxRetries int

	Output       io.Writer
	FollowOutput io.Writer
}

func RunSubmit(ctx context.Context, opts SubmitOptions) error {
	out := opts.Output
	if out == nil {
		out = io.Discard
	}
	followOut := opts.FollowOutput
	if followOut == nil {
		followOut = out
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	specPayload, err := resolveRunSubmitSpecPayload(ctx, base, httpClient, opts.SpecPath)
	if err != nil {
		return err
	}

	repo, err := resolveSourceRepo(ctx, base, httpClient, opts.RepoSelector)
	if err != nil {
		return err
	}
	if opts.Apply && !repo.IsLocal {
		return errors.New("--apply requires a local repo")
	}

	request := domainapi.RunSubmitRequest{
		RepoURL:   domaintypes.RepoURL(repo.RepoURL),
		Ref:       domaintypes.GitRef(repo.Ref),
		CommitSHA: repo.CommitSHA,
		Spec:      specPayload.Spec,
		CreatedBy: strings.TrimSpace(os.Getenv("USER")),
	}

	runID, migID, err := submitSingleRepoRun(ctx, base, httpClient, request)
	if err != nil {
		return err
	}

	needsFinal := opts.Follow || opts.PullArtifacts || opts.Apply
	if !needsFinal {
		_, _ = fmt.Fprintf(out, "run_id: %s\n", runID.String())
		_, _ = fmt.Fprintf(out, "mig_id: %s\n", migID.String())
		return nil
	}

	final, err := followRunStatusReports(ctx, base, httpClient, runID, followOut, specPayload.DisplayName, opts.MaxRetries, time.Second)
	if err != nil {
		return err
	}
	if final != migsapi.RunStateSucceeded {
		return fmt.Errorf("run ended in %s", strings.ToLower(string(final)))
	}

	if opts.PullArtifacts {
		artifactDir, err := resolveArtifactOutputDir(opts.PullPath)
		if err != nil {
			return err
		}
		if err := DownloadRunArtifacts(ctx, base, httpClient, runID.String(), artifactDir, followOut); err != nil {
			return err
		}
	}

	if opts.Apply {
		return runApply(ctx, ApplyOptions{
			RunID:    runID.String(),
			RepoPath: repo.Worktree,
			Output:   followOut,
		}, base, httpClient)
	}
	return nil
}

type runSubmitSpecPayload struct {
	Spec        json.RawMessage
	DisplayName string
}

func resolveRunSubmitSpecPayload(ctx context.Context, base *url.URL, client *http.Client, specArg string) (runSubmitSpecPayload, error) {
	specArg = strings.TrimSpace(specArg)
	if specArg == "" {
		return runSubmitSpecPayload{}, errors.New("spec path required")
	}
	if info, err := os.Stat(specArg); err == nil {
		specPath, err := normalizeRunSpecPath(specArg, info)
		if err != nil {
			return runSubmitSpecPayload{}, err
		}
		spec, err := buildRunSubmitSpecPayload(ctx, base, client, specPath, "")
		return runSubmitSpecPayload{Spec: spec}, err
	}

	localPath, stepSelector, ok, err := splitLocalRunSpecSelector(specArg)
	if err != nil {
		return runSubmitSpecPayload{}, err
	}
	if ok {
		specPath, err := resolveRunSpecPath(localPath)
		if err != nil {
			return runSubmitSpecPayload{}, err
		}
		spec, err := buildRunSubmitSpecPayload(ctx, base, client, specPath, stepSelector)
		return runSubmitSpecPayload{Spec: spec}, err
	}

	if pathExplicitlyLocal(specArg) {
		specPath, err := resolveRunSpecPath(specArg)
		if err != nil {
			return runSubmitSpecPayload{}, err
		}
		spec, err := buildRunSubmitSpecPayload(ctx, base, client, specPath, "")
		return runSubmitSpecPayload{Spec: spec}, err
	}

	return resolveNamedRunSubmitSpecPayload(ctx, base, client, specArg)
}

func splitLocalRunSpecSelector(specArg string) (string, string, bool, error) {
	if _, err := os.Stat(specArg); err == nil {
		return specArg, "", true, nil
	}
	idx := strings.LastIndex(specArg, ":")
	if idx < 0 {
		return "", "", false, nil
	}
	specPath := strings.TrimSpace(specArg[:idx])
	stepSelector := strings.TrimSpace(specArg[idx+1:])
	if !pathExplicitlyLocal(specPath) {
		if _, err := os.Stat(specPath); err != nil {
			return "", "", false, nil
		}
	}
	if specPath == "" {
		return "", "", true, errors.New("spec path required")
	}
	if stepSelector == "" {
		return "", "", true, errors.New("step name required")
	}
	return specPath, stepSelector, true, nil
}

func pathExplicitlyLocal(path string) bool {
	return strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") || filepath.IsAbs(path)
}

func resolveRunSpecPath(specPath string) (string, error) {
	specPath = strings.TrimSpace(specPath)
	if specPath == "" {
		return "", errors.New("spec path required")
	}
	info, err := os.Stat(specPath)
	if err != nil {
		return "", fmt.Errorf("load spec: %w", err)
	}
	return normalizeRunSpecPath(specPath, info)
}

func normalizeRunSpecPath(specPath string, info os.FileInfo) (string, error) {
	if info.IsDir() {
		specPath = filepath.Join(specPath, "mig.yaml")
		if _, err := os.Stat(specPath); err != nil {
			return "", fmt.Errorf("load spec: %w", err)
		}
	}
	return specPath, nil
}

func resolveNamedRunSubmitSpecPayload(ctx context.Context, base *url.URL, client *http.Client, selector string) (runSubmitSpecPayload, error) {
	if base == nil {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: base url required")
	}
	if client == nil {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: http client required")
	}

	endpoint := base.JoinPath("v1", "specs", "resolve")
	query := endpoint.Query()
	query.Set("selector", selector)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: resolve named spec: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: resolve named spec: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: named spec not found: %s", selector)
	case http.StatusBadRequest, http.StatusConflict:
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: %s", httpx.ReadErrorMessage(resp.Body, resp.Status, httpx.MaxErrorBodyBytes))
	default:
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: resolve named spec: %s", httpx.ReadErrorMessage(resp.Body, resp.Status, httpx.MaxErrorBodyBytes))
	}

	var resolved domainapi.NamedSpecResolveResponse
	if err := httpx.DecodeResponseJSON(resp.Body, &resolved, httpx.MaxJSONBodyBytes); err != nil {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: resolve named spec: decode response: %w", err)
	}
	if len(resolved.Spec) == 0 {
		return runSubmitSpecPayload{}, fmt.Errorf("run submit: resolve named spec: empty spec in response")
	}
	return runSubmitSpecPayload{
		Spec:        resolved.Spec,
		DisplayName: namedSpecDisplayName(resolved),
	}, nil
}

func namedSpecDisplayName(resolved domainapi.NamedSpecResolveResponse) string {
	domain := strings.Trim(strings.TrimSpace(resolved.Source.Domain), "/")
	repo := strings.Trim(strings.TrimSpace(resolved.Source.Repo), "/")
	name := strings.TrimSpace(resolved.Name)
	if domain == "" || repo == "" || name == "" {
		return ""
	}
	return domain + "/" + repo + ":" + name
}

func buildRunSubmitSpecPayload(ctx context.Context, base *url.URL, client *http.Client, specPath string, stepSelector string) (json.RawMessage, error) {
	specPayload, err := specpayload.BuildSelected(
		ctx,
		base,
		client,
		specPath,
		stepSelector,
		nil,
		"",
		false,
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	if len(specPayload) == 0 {
		return nil, fmt.Errorf("load spec: spec is empty")
	}
	return specPayload, nil
}

func followRunStatusReports(ctx context.Context, baseURL *url.URL, client *http.Client, runID domaintypes.RunID, out io.Writer, specDisplayName string, maxRetries int, pollInterval time.Duration) (migsapi.RunState, error) {
	renderOpts := common.FollowRunRenderOptions(baseURL, out)
	renderOpts.SpecDisplayName = specDisplayName
	if maxRetries == 0 {
		maxRetries = 5
	}
	return runs.FollowRunCommand{
		Client:          client,
		BaseURL:         baseURL,
		RunID:           runID,
		Output:          out,
		EnableOSC8:      renderOpts.EnableOSC8,
		AuthToken:       renderOpts.AuthToken,
		SpecDisplayName: renderOpts.SpecDisplayName,
		MaxRetries:      maxRetries,
		PollInterval:    pollInterval,
	}.Run(ctx)
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

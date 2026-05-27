package mig

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
	"strings"
	"text/tabwriter"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/migs"
	runcli "github.com/iw2rmb/ploy/internal/cli/run"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	"github.com/iw2rmb/ploy/internal/cli/specpayload"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

type AddOptions struct {
	Name     string
	SpecPath string
	Output   io.Writer
}

func RunAdd(ctx context.Context, opts AddOptions) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	var specData *json.RawMessage
	if strings.TrimSpace(opts.SpecPath) != "" {
		data, err := specpayload.Load(ctx, base, httpClient, opts.SpecPath)
		if err != nil {
			return fmt.Errorf("load spec: %w", err)
		}
		specData = &data
	}

	result, err := migs.AddMigCommand{
		Client:  httpClient,
		BaseURL: base,
		Name:    opts.Name,
		Spec:    specData,
	}.Run(ctx)
	if err != nil {
		return err
	}

	if result.SpecID != nil {
		_, _ = fmt.Fprintf(opts.Output, "Mig created: %s (name: %s, spec_id: %s)\n", result.ID.String(), result.Name, result.SpecID.String())
	} else {
		_, _ = fmt.Fprintf(opts.Output, "Mig created: %s (name: %s)\n", result.ID.String(), result.Name)
	}
	return nil
}

func RunList(ctx context.Context, output io.Writer) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	results, err := migs.ListMigsCommand{Client: httpClient, BaseURL: base, Limit: 100}.Run(ctx)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		_, _ = fmt.Fprintln(output, "No migs found.")
		return nil
	}

	w := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tCREATED_AT\tARCHIVED")
	for _, mig := range results {
		archived := "-"
		if mig.Archived {
			archived = "yes"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", mig.ID.String(), mig.Name, mig.CreatedAt.Format(time.RFC3339), archived)
	}
	return w.Flush()
}

func RunRemove(ctx context.Context, migRef string, output io.Writer) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	if err := (migs.RemoveMigCommand{Client: httpClient, BaseURL: base, MigRef: domaintypes.MigRef(migRef)}).Run(ctx); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "Mig deleted: %s\n", migRef)
	return nil
}

func RunArchive(ctx context.Context, migRef string, output io.Writer) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	result, err := (migs.ArchiveMigCommand{Client: httpClient, BaseURL: base, MigRef: domaintypes.MigRef(migRef)}).Run(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "Mig archived: %s (name: %s)\n", result.ID.String(), result.Name)
	return nil
}

func RunUnarchive(ctx context.Context, migRef string, output io.Writer) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	result, err := (migs.UnarchiveMigCommand{Client: httpClient, BaseURL: base, MigRef: domaintypes.MigRef(migRef)}).Run(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "Mig unarchived: %s (name: %s)\n", result.ID.String(), result.Name)
	return nil
}

func RunSpecSet(ctx context.Context, migRef, specPath string, output io.Writer) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	specData, err := specpayload.Load(ctx, base, httpClient, specPath)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}
	result, err := (migs.SetMigSpecCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migRef),
		Spec:    specData,
	}).Run(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "Spec set: %s (created_at: %s)\n", result.ID.String(), result.CreatedAt.Format(time.RFC3339))
	return nil
}

type RepoAddOptions struct {
	MigRef    string
	RepoURL   string
	BaseRef   string
	TargetRef string
	Output    io.Writer
}

func RunRepoAdd(ctx context.Context, opts RepoAddOptions) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	migID, err := resolveMigRef(ctx, httpClient, base, opts.MigRef)
	if err != nil {
		return err
	}
	result, err := (migs.AddMigRepoCommand{
		Client:    httpClient,
		BaseURL:   base,
		MigRef:    domaintypes.MigRef(migID),
		RepoURL:   opts.RepoURL,
		BaseRef:   opts.BaseRef,
		TargetRef: opts.TargetRef,
	}).Run(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(opts.Output, "Repo added: %s (repo_id: %s)\n", domaintypes.NormalizeRepoURLSchemless(result.RepoURL), result.ID.String())
	return nil
}

func RunRepoList(ctx context.Context, migRef string, output io.Writer) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	migID, err := resolveMigRef(ctx, httpClient, base, migRef)
	if err != nil {
		return err
	}
	results, err := (migs.ListMigReposCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migID),
	}).Run(ctx)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		_, _ = fmt.Fprintln(output, "No repos found.")
		return nil
	}

	w := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tREPO_URL\tBASE_REF\tTARGET_REF\tADDED_AT")
	for _, repo := range results {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			repo.ID.String(),
			domaintypes.NormalizeRepoURLSchemless(repo.RepoURL),
			repo.BaseRef,
			repo.TargetRef,
			repo.CreatedAt.Format(time.RFC3339),
		)
	}
	return w.Flush()
}

func RunRepoRemove(ctx context.Context, migRef, repoID string, output io.Writer) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	migID, err := resolveMigRef(ctx, httpClient, base, migRef)
	if err != nil {
		return err
	}
	if err := (migs.RemoveMigRepoCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migID),
		RepoID:  domaintypes.MigRepoID(repoID),
	}).Run(ctx); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "Repo deleted: %s\n", repoID)
	return nil
}

func RunRepoImport(ctx context.Context, migRef, filePath string, output io.Writer) error {
	csvData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read csv file: %w", err)
	}
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	migID, err := resolveMigRef(ctx, httpClient, base, migRef)
	if err != nil {
		return err
	}
	result, err := (migs.ImportMigReposCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migID),
		CSVData: csvData,
	}).Run(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "Import complete: %d created, %d updated, %d failed\n", result.Created, result.Updated, result.Failed)
	for _, e := range result.Errors {
		_, _ = fmt.Fprintf(output, "  Line %d: %s\n", e.Line, e.Message)
	}
	return nil
}

type RunOptions struct {
	MigRef      string
	RepoURLs    []string
	Failed      bool
	Follow      bool
	Cap         time.Duration
	CancelOnCap bool
	MaxRetries  int
	Output      io.Writer
}

func RunProject(ctx context.Context, opts RunOptions) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	migID, err := resolveMigRef(ctx, httpClient, base, opts.MigRef)
	if err != nil {
		return err
	}
	result, err := (migs.CreateMigRunCommand{
		Client:   httpClient,
		BaseURL:  base,
		MigRef:   domaintypes.MigRef(migID),
		RepoURLs: opts.RepoURLs,
		Failed:   opts.Failed,
	}).Run(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(opts.Output, result.RunID)

	if opts.Follow {
		return followMigRunProject(ctx, base, httpClient, result.RunID, opts.Cap, opts.CancelOnCap, opts.MaxRetries, opts.Output)
	}
	return nil
}

func RunFetch(ctx context.Context, runID, artifactDir string, output io.Writer) error {
	if strings.TrimSpace(runID) == "" {
		return errors.New("run id required")
	}
	if strings.TrimSpace(artifactDir) == "" {
		return errors.New("artifact-dir required")
	}
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	return runcli.DownloadRunArtifacts(ctx, base, httpClient, strings.TrimSpace(runID), strings.TrimSpace(artifactDir), output)
}

func RunArtifacts(ctx context.Context, runID string, output io.Writer) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	return (migs.ArtifactsCommand{
		BaseURL: base,
		Client:  httpClient,
		RunID:   domaintypes.RunID(strings.TrimSpace(runID)),
		Output:  output,
	}).Run(ctx)
}

func RunStatus(ctx context.Context, migID string, output io.Writer) error {
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	migID = strings.TrimSpace(migID)

	migSummary, err := findMigByID(ctx, httpClient, base, migID)
	if err != nil {
		return err
	}
	repos, err := (migs.ListMigReposCommand{Client: httpClient, BaseURL: base, MigRef: domaintypes.MigRef(migID)}).Run(ctx)
	if err != nil {
		return err
	}
	runs, err := listRunsByMigID(ctx, httpClient, base, migID)
	if err != nil {
		return err
	}

	specID := "-"
	if migSummary.SpecID != nil && !migSummary.SpecID.IsZero() {
		specID = migSummary.SpecID.String()
	}

	_, _ = fmt.Fprintf(output, "Mig:   %s  | %s\n", migSummary.ID.String(), migValueOrDash(strings.TrimSpace(migSummary.Name)))
	_, _ = fmt.Fprintf(output, "Spec:  %s | Download\n", specID)
	_, _ = fmt.Fprintf(output, "Repos: %d\n", len(repos))
	_, _ = fmt.Fprintln(output, "")
	if len(runs) == 0 {
		_, _ = fmt.Fprintln(output, "No runs found for this migration.")
		return nil
	}

	tw := tabwriter.NewWriter(output, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "Run\tSuccess\tFail")
	for _, run := range runs {
		success, fail := runSuccessFail(run.Counts)
		_, _ = fmt.Fprintf(tw, "%s  %s\t%d\t%d\n", migStatusGlyph(run.Status.String()), run.ID.String(), success, fail)
	}
	return tw.Flush()
}

type RunRepoAddOptions struct {
	RunID     string
	RepoURL   string
	BaseRef   string
	TargetRef string
	Output    io.Writer
}

func RunRunRepoAdd(ctx context.Context, opts RunRepoAddOptions) error {
	if strings.TrimSpace(opts.RunID) == "" {
		return errors.New("run-id required")
	}
	trimmedRepoURL := strings.TrimSpace(opts.RepoURL)
	if trimmedRepoURL == "" {
		return errors.New("--repo-url required")
	}
	if err := domaintypes.RepoURL(trimmedRepoURL).Validate(); err != nil {
		return fmt.Errorf("--repo-url: %w", err)
	}
	baseRef := strings.TrimSpace(opts.BaseRef)
	if baseRef == "" {
		return errors.New("--base-ref required")
	}
	if err := domaintypes.GitRef(baseRef).Validate(); err != nil {
		return fmt.Errorf("--base-ref: %w", err)
	}
	targetRef := strings.TrimSpace(opts.TargetRef)
	if targetRef == "" {
		return errors.New("--target-ref required")
	}
	if err := domaintypes.GitRef(targetRef).Validate(); err != nil {
		return fmt.Errorf("--target-ref: %w", err)
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	resp, err := doRunRepoAdd(ctx, base, httpClient, strings.TrimSpace(opts.RunID), runRepoAddRequest{
		RepoURL:   trimmedRepoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(opts.Output, "Repo added: %s (repo_id: %s, status: %s)\n", domaintypes.NormalizeRepoURLSchemless(resp.RepoURL), resp.RepoID, resp.Status)
	return nil
}

func RunRunRepoRemove(ctx context.Context, runID, repoID string, output io.Writer) error {
	if strings.TrimSpace(runID) == "" {
		return errors.New("run-id required")
	}
	if strings.TrimSpace(repoID) == "" {
		return errors.New("--repo-id required")
	}
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	resp, err := doRunRepoRemove(ctx, base, httpClient, strings.TrimSpace(runID), strings.TrimSpace(repoID))
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "Repo removed: %s (repo_id: %s, status: %s)\n", domaintypes.NormalizeRepoURLSchemless(resp.RepoURL), resp.RepoID, resp.Status)
	return nil
}

type RunRepoRestartOptions struct {
	RunID     string
	RepoID    string
	BaseRef   string
	TargetRef string
	Output    io.Writer
}

func RunRunRepoRestart(ctx context.Context, opts RunRepoRestartOptions) error {
	if strings.TrimSpace(opts.RunID) == "" {
		return errors.New("run-id required")
	}
	if strings.TrimSpace(opts.RepoID) == "" {
		return errors.New("--repo-id required")
	}
	reqBody := runRepoRestartRequest{}
	if br := strings.TrimSpace(opts.BaseRef); br != "" {
		reqBody.BaseRef = &br
	}
	if tr := strings.TrimSpace(opts.TargetRef); tr != "" {
		reqBody.TargetRef = &tr
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	resp, err := doRunRepoRestart(ctx, base, httpClient, strings.TrimSpace(opts.RunID), strings.TrimSpace(opts.RepoID), reqBody)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(opts.Output, "Repo restarted: %s (repo_id: %s, attempt: %d, status: %s)\n", domaintypes.NormalizeRepoURLSchemless(resp.RepoURL), resp.RepoID, resp.Attempt, resp.Status)
	return nil
}

func resolveMigRef(ctx context.Context, client *http.Client, baseURL *url.URL, migRef string) (domaintypes.MigID, error) {
	id, err := (migs.ResolveMigByNameCommand{
		Client:  client,
		BaseURL: baseURL,
		MigRef:  domaintypes.MigRef(migRef),
	}).Run(ctx)
	return domaintypes.MigID(id), err
}

func followMigRunProject(ctx context.Context, baseURL *url.URL, client *http.Client, runID domaintypes.RunID, capDuration time.Duration, cancelOnCap bool, maxRetries int, output io.Writer) error {
	followCtx := ctx
	var cancel context.CancelFunc
	if capDuration > 0 {
		followCtx, cancel = context.WithTimeout(ctx, capDuration)
		defer cancel()
	}
	renderOpts := common.FollowRunRenderOptions(baseURL, output)
	final, err := runs.FollowRunCommand{
		Client:     common.CloneForStream(client),
		BaseURL:    baseURL,
		RunID:      runID,
		Output:     output,
		EnableOSC8: renderOpts.EnableOSC8,
		AuthToken:  renderOpts.AuthToken,
		MaxRetries: maxRetries,
	}.Run(followCtx)
	if err != nil {
		if capDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
			if cancelOnCap {
				_, _ = fmt.Fprintln(output, "Follow timed out; requesting run cancellation...")
				_ = runs.CancelCommand{
					BaseURL: baseURL,
					Client:  client,
					RunID:   runID,
					Reason:  "cap exceeded",
					Output:  output,
				}.Run(context.Background())
			} else {
				_, _ = fmt.Fprintf(output, "Follow capped after %s; run %s continues running in the background.\n", capDuration.String(), runID)
			}
			return nil
		}
		return err
	}
	if final != migsapi.RunStateSucceeded {
		return fmt.Errorf("mig run ended in %s", strings.ToLower(string(final)))
	}
	return nil
}

func findMigByID(ctx context.Context, httpClient *http.Client, baseURL *url.URL, migID string) (domainapi.MigSummary, error) {
	const pageSize int32 = 100
	var offset int32
	for {
		page, err := migs.ListMigsCommand{
			Client:  httpClient,
			BaseURL: baseURL,
			Limit:   pageSize,
			Offset:  offset,
		}.Run(ctx)
		if err != nil {
			return domainapi.MigSummary{}, err
		}
		for _, item := range page {
			if item.ID.String() == migID {
				return item, nil
			}
		}
		if len(page) < int(pageSize) {
			return domainapi.MigSummary{}, fmt.Errorf("mig %q not found", migID)
		}
		offset += pageSize
	}
}

func listRunsByMigID(ctx context.Context, httpClient *http.Client, baseURL *url.URL, migID string) ([]domaintypes.RunSummary, error) {
	const pageSize int32 = 100
	var offset int32
	result := make([]domaintypes.RunSummary, 0)
	for {
		page, err := migs.ListBatchesCommand{
			Client:  httpClient,
			BaseURL: baseURL,
			Limit:   pageSize,
			Offset:  offset,
		}.Run(ctx)
		if err != nil {
			return nil, err
		}
		for _, run := range page {
			if run.MigID.String() == migID {
				result = append(result, run)
			}
		}
		if len(page) < int(pageSize) {
			break
		}
		offset += pageSize
	}
	return result, nil
}

func runSuccessFail(counts *domaintypes.RunRepoCounts) (int32, int32) {
	if counts == nil {
		return 0, 0
	}
	return counts.Success, counts.Fail
}

func migStatusGlyph(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "started":
		return "⣽"
	case "success", "succeeded":
		return "✓"
	case "fail", "failed":
		return "✗"
	case "cancelled", "canceled":
		return "○"
	case "queued", "created":
		return "·"
	default:
		return " "
	}
}

func migValueOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

// runRepoAddRequest is the request body for adding a repo to a batch.
type runRepoAddRequest struct {
	RepoURL   string `json:"repo_url"`
	BaseRef   string `json:"base_ref"`
	TargetRef string `json:"target_ref"`
}

type runRepoRestartRequest struct {
	BaseRef   *string `json:"base_ref,omitempty"`
	TargetRef *string `json:"target_ref,omitempty"`
}

type runRepoResponse struct {
	RunID      domaintypes.RunID     `json:"run_id"`
	RepoID     domaintypes.MigRepoID `json:"repo_id"`
	RepoURL    string                `json:"repo_url"`
	BaseRef    string                `json:"base_ref"`
	TargetRef  string                `json:"target_ref"`
	Status     string                `json:"status"`
	Attempt    int32                 `json:"attempt"`
	LastError  *string               `json:"last_error,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	StartedAt  *time.Time            `json:"started_at,omitempty"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
}

func doRunRepoAdd(ctx context.Context, base *url.URL, client *http.Client, batchID string, req runRepoAddRequest) (runRepoResponse, error) {
	endpoint := base.JoinPath("v1", "runs", batchID, "repos")
	body, err := json.Marshal(req)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return runRepoResponse{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	var result runRepoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return runRepoResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

func doRunRepoRemove(ctx context.Context, base *url.URL, client *http.Client, batchID, repoID string) (runRepoResponse, error) {
	endpoint := base.JoinPath("v1", "runs", batchID, "repos", repoID, "cancel")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("create request: %w", err)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return runRepoResponse{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	var result runRepoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return runRepoResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

func doRunRepoRestart(ctx context.Context, base *url.URL, client *http.Client, batchID, repoID string, req runRepoRestartRequest) (runRepoResponse, error) {
	endpoint := base.JoinPath("v1", "runs", batchID, "repos", repoID, "restart")
	body, err := json.Marshal(req)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return runRepoResponse{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return runRepoResponse{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	var result runRepoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return runRepoResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

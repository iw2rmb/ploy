package main

import (
    "context"
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "os"
    "path/filepath"
    "strings"
    "time"

    gonanoid "github.com/matoous/go-nanoid/v2"

    artifactcli "github.com/iw2rmb/ploy/internal/cli/artifact"
    "github.com/iw2rmb/ploy/internal/cli/mods"
    "github.com/iw2rmb/ploy/internal/cli/stream"
    modsapi "github.com/iw2rmb/ploy/internal/mods/api"
    modplan "github.com/iw2rmb/ploy/internal/mods/plan"
)

// handleModRun executes the Mods-specific run command.
func handleModRun(args []string, stderr io.Writer) error {
	return executeModRun(args, stderr)
}

func executeModRun(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("mod run", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    ticket := fs.String("ticket", "auto", "ticket identifier to consume or 'auto'")
    tenant := fs.String("tenant", "", "tenant slug for subject mapping")
    repoURL := fs.String("repo-url", "", "Git repository URL to materialise for Mods execution")
    repoBaseRef := fs.String("repo-base-ref", "", "Git base ref used for materialisation")
    repoTargetRef := fs.String("repo-target-ref", "", "Git target ref created for the run")
    repoWorkspace := fs.String("repo-workspace-hint", "", "Optional subdirectory hint when preparing the workspace")
    follow := fs.Bool("follow", false, "follow ticket events until completion")
    artifactDir := fs.String("artifact-dir", "", "directory to download final artifacts into (with manifest.json)")
    maxRetries := fs.Int("max-retries", 5, "max reconnect attempts for event stream (-1 for unlimited)")
    retryWait := fs.Duration("retry-wait", 500*time.Millisecond, "wait between event stream reconnects")
    if err := fs.Parse(args); err != nil {
        printModRunUsage(stderr)
        return err
    }

	trimmedTenant := strings.TrimSpace(*tenant)
	if trimmedTenant == "" {
		printModRunUsage(stderr)
		return errors.New("tenant required")
	}

	ticketValue := strings.TrimSpace(*ticket)
	if ticketValue == "" || strings.EqualFold(ticketValue, "auto") {
		generated, err := gonanoid.Generate("abcdefghijklmnopqrstuvwxyz0123456789", 12)
		if err != nil {
			return fmt.Errorf("generate ticket id: %w", err)
		}
		ticketValue = fmt.Sprintf("mods-%s", generated)
	}

	repoSpec := struct {
		URL           string
		BaseRef       string
		TargetRef     string
		WorkspaceHint string
	}{
		URL:           strings.TrimSpace(*repoURL),
		BaseRef:       strings.TrimSpace(*repoBaseRef),
		TargetRef:     strings.TrimSpace(*repoTargetRef),
		WorkspaceHint: strings.TrimSpace(*repoWorkspace),
	}
	if repoSpec.URL != "" && repoSpec.TargetRef == "" {
		printModRunUsage(stderr)
		return fmt.Errorf("repo target ref required when repo url is set")
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	request := modsapi.TicketSubmitRequest{
		TicketID:   ticketValue,
		Tenant:     trimmedTenant,
		Submitter:  strings.TrimSpace(os.Getenv("USER")),
		Repository: repoSpec.URL,
		Metadata:   make(map[string]string),
		Stages:     defaultStageDefinitions(),
	}
	if repoSpec.BaseRef != "" {
		request.Metadata["repo_base_ref"] = repoSpec.BaseRef
	}
	if repoSpec.TargetRef != "" {
		request.Metadata["repo_target_ref"] = repoSpec.TargetRef
	}
	if repoSpec.WorkspaceHint != "" {
		request.Metadata["repo_workspace_hint"] = repoSpec.WorkspaceHint
	}

    cmd := mods.SubmitCommand{
        Client:  httpClient,
        BaseURL: base,
        Request: request,
    }
    summary, err := cmd.Run(ctx)
    if err != nil {
        return err
    }
    _, _ = fmt.Fprintf(stderr, "Mods ticket %s submitted (state: %s)\n", summary.TicketID, summary.State)

    if *follow {
        ev := mods.EventsCommand{
            Client: stream.Client{
                HTTPClient:   httpClient,
                MaxRetries:   *maxRetries,
                RetryBackoff: *retryWait,
            },
            BaseURL: base,
            Ticket:  summary.TicketID,
            Output:  stderr,
        }
        final, err := ev.Run(ctx)
        if err != nil {
            return err
        }
        _, _ = fmt.Fprintf(stderr, "Final state: %s\n", strings.ToLower(string(final)))
        if final != modsapi.TicketStateSucceeded {
            return fmt.Errorf("mod run ended in %s", strings.ToLower(string(final)))
        }
        if strings.TrimSpace(*artifactDir) != "" {
            if err := downloadTicketArtifacts(ctx, base, httpClient, summary.TicketID, strings.TrimSpace(*artifactDir), stderr); err != nil {
                return err
            }
        }
    }
    return nil
}

func printModRunUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy mod run --tenant <tenant> [--ticket <ticket-id>|--ticket auto] [--repo-url <url> --repo-base-ref <branch> --repo-target-ref <branch> --repo-workspace-hint <dir>] [--follow] [--artifact-dir <dir>] [--max-retries N] [--retry-wait D]")
}

func defaultStageDefinitions() []modsapi.StageDefinition {
	return []modsapi.StageDefinition{
		{ID: modplan.StageNamePlan, Lane: "mods-plan", Priority: "default", MaxAttempts: 1},
		{ID: modplan.StageNameORWApply, Lane: "mods-java", Priority: "default", MaxAttempts: 1, Dependencies: []string{modplan.StageNamePlan}},
		{ID: modplan.StageNameORWGenerate, Lane: "mods-java", Priority: "default", MaxAttempts: 1, Dependencies: []string{modplan.StageNamePlan}},
		{ID: modplan.StageNameLLMPlan, Lane: "mods-llm", Priority: "default", MaxAttempts: 1, Dependencies: []string{modplan.StageNamePlan}},
		{ID: modplan.StageNameLLMExec, Lane: "mods-llm", Priority: "default", MaxAttempts: 1, Dependencies: []string{modplan.StageNameORWApply, modplan.StageNameORWGenerate, modplan.StageNameLLMPlan}},
		{ID: modplan.StageNameHuman, Lane: "mods-human", Priority: "default", MaxAttempts: 1, Dependencies: []string{modplan.StageNameLLMExec}},
	}
}

// downloadTicketArtifacts fetches ticket status and downloads referenced artifacts into dir.
func downloadTicketArtifacts(ctx context.Context, base *url.URL, httpClient *http.Client, ticketID, dir string, out io.Writer) error {
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return fmt.Errorf("create artifact dir %s: %w", dir, err)
    }
    // Fetch ticket status
    statusURL, err := url.JoinPath(base.String(), "v1", "mods", url.PathEscape(strings.TrimSpace(ticketID)))
    if err != nil { return err }
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
    if err != nil { return fmt.Errorf("build status request: %w", err) }
    resp, err := httpClient.Do(req)
    if err != nil { return fmt.Errorf("fetch ticket status: %w", err) }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return controlPlaneHTTPError(resp)
    }
    var payload modsapi.TicketStatusResponse
    if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
        return fmt.Errorf("decode ticket status: %w", err)
    }
    // Collect artifacts
    type manifestItem struct {
        Stage  string `json:"stage"`
        Name   string `json:"name"`
        CID    string `json:"cid"`
        Digest string `json:"digest"`
        Size   int64  `json:"size"`
        Path   string `json:"path"`
    }
    items := make([]manifestItem, 0)
    svc, err := artifactClientFactory()
    if err != nil { return err }
    var downloaded int
    for stageID, st := range payload.Ticket.Stages {
        for name, cid := range st.Artifacts {
            res, err := artifactcli.Pull(ctx, func() (artifactcli.Service, error) { return svc, nil }, cid)
            if err != nil { return err }
            filename := buildArtifactFilename(stageID, name, cid, res.Digest)
            path := filepath.Join(dir, filename)
            if err := os.WriteFile(path, res.Data, 0o644); err != nil {
                return fmt.Errorf("write artifact %s: %w", filename, err)
            }
            items = append(items, manifestItem{Stage: stageID, Name: name, CID: cid, Digest: res.Digest, Size: res.Size, Path: path})
            downloaded++
        }
    }
    // Write manifest
    manifestPath := filepath.Join(dir, "manifest.json")
    data, _ := json.MarshalIndent(struct{ Artifacts []manifestItem `json:"artifacts"`}{Artifacts: items}, "", "  ")
    if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
        return fmt.Errorf("write manifest: %w", err)
    }
    _, _ = fmt.Fprintf(out, "Downloaded %d artifacts to %s\n", downloaded, dir)
    return nil
}

func buildArtifactFilename(stage, name, cid, digest string) string {
    clean := func(s string) string {
        s = strings.TrimSpace(s)
        s = strings.ReplaceAll(s, "/", "_")
        s = strings.ReplaceAll(s, "\\", "_")
        return s
    }
    stage = clean(stage)
    name = clean(name)
    cid = clean(cid)
    if d := strings.TrimSpace(digest); d != "" {
        d = strings.ReplaceAll(d, ":", "-")
        if len(d) > 20 { d = d[:20] }
        return fmt.Sprintf("%s_%s_%s.bin", d, stage, name)
    }
    return fmt.Sprintf("%s_%s_%s.bin", cid, stage, name)
}

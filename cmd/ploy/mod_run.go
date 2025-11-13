package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	modplan "github.com/iw2rmb/ploy/internal/workflow/mods/plan"
)

// handleModRun executes the Mods-specific run command.
func handleModRun(args []string, stderr io.Writer) error {
	return executeModRun(args, stderr)
}

func executeModRun(args []string, stderr io.Writer) error {
	// Parse CLI flags using extracted flag handling
	flags, err := parseModRunFlags(args)
	if err != nil {
		printModRunUsage(stderr)
		return err
	}

	// Build repository specification from parsed flags
	repoSpec := struct {
		URL           string
		BaseRef       string
		TargetRef     string
		WorkspaceHint string
	}{
		URL:           strings.TrimSpace(*flags.RepoURL),
		BaseRef:       strings.TrimSpace(*flags.RepoBaseRef),
		TargetRef:     strings.TrimSpace(*flags.RepoTargetRef),
		WorkspaceHint: strings.TrimSpace(*flags.RepoWorkspaceHint),
	}
	if repoSpec.URL != "" && repoSpec.TargetRef == "" {
		printModRunUsage(stderr)
		return fmt.Errorf("repo target ref required when repo url is set")
	}

	// Note: validation of GitLab flags is deferred to the server/node.
	// Avoid empty branches and keep CLI permissive for per-run overrides.

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	request := modsapi.TicketSubmitRequest{
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

	// Load spec from file (if provided) and merge with CLI overrides.
	// CLI flags take precedence over spec file values.
	specPayload, err := buildSpecPayload(
		strings.TrimSpace(*flags.SpecFile),
		*flags.ModEnvs,
		strings.TrimSpace(*flags.ModImage),
		*flags.Retain,
		strings.TrimSpace(*flags.ModCommand),
		strings.TrimSpace(*flags.GitLabPAT),
		strings.TrimSpace(*flags.GitLabDomain),
		*flags.MRSuccess,
		*flags.MRFail,
		*flags.HealOnBuild,
	)
	if err != nil {
		return fmt.Errorf("build spec: %w", err)
	}

	cmd := mods.SubmitCommand{
		Client:  httpClient,
		BaseURL: base,
		Request: request,
		Spec:    specPayload,
	}
	summary, err := cmd.Run(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stderr, "Mods ticket %s submitted (state: %s)\n", summary.TicketID, summary.State)

	initialState := strings.ToLower(string(summary.State))
	finalState := ""

	// Follow ticket events if requested
	if *flags.Follow {
		followCtx := ctx
		var cancel context.CancelFunc
		if *flags.CapDuration > 0 {
			followCtx, cancel = context.WithTimeout(ctx, *flags.CapDuration)
			defer cancel()
		}
		ev := mods.EventsCommand{
			Client: stream.Client{
				HTTPClient:   cloneForStream(httpClient),
				MaxRetries:   *flags.MaxRetries,
				RetryBackoff: *flags.RetryWait,
			},
			BaseURL: base,
			Ticket:  string(summary.TicketID),
			Output:  stderr,
		}
		final, err := ev.Run(followCtx)
		if err != nil {
			if *flags.CapDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
				if *flags.CancelOnCap {
					_, _ = fmt.Fprintln(stderr, "Follow timed out; requesting ticket cancellation...")
					_ = mods.CancelCommand{BaseURL: base, Client: httpClient, Ticket: string(summary.TicketID), Reason: "cap exceeded", Output: stderr}.Run(context.Background())
				} else {
					_, _ = fmt.Fprintf(stderr, "Follow capped after %s; ticket %s continues running in the background.\n", flags.CapDuration.String(), summary.TicketID)
				}
				return nil
			}
			return err
		}
		_, _ = fmt.Fprintf(stderr, "Final state: %s\n", strings.ToLower(string(final)))
		if final != modsapi.TicketStateSucceeded {
			return fmt.Errorf("mod run ended in %s", strings.ToLower(string(final)))
		}
		finalState = strings.ToLower(string(final))
		if strings.TrimSpace(*flags.ArtifactDir) != "" {
			if err := downloadTicketArtifacts(ctx, base, httpClient, string(summary.TicketID), strings.TrimSpace(*flags.ArtifactDir), stderr); err != nil {
				return err
			}
		}
	}

	// Output JSON summary if requested
	if *flags.JSONOut {
		// Optional: probe MR URL from ticket status metadata.
		mrURL, _ := fetchMRURL(ctx, base, httpClient, string(summary.TicketID))
		type runJSON struct {
			TicketID    string `json:"ticket_id"`
			Initial     string `json:"initial_state,omitempty"`
			Final       string `json:"final_state,omitempty"`
			ArtifactDir string `json:"artifact_dir,omitempty"`
			MRURL       string `json:"mr_url,omitempty"`
		}
		out := runJSON{TicketID: string(summary.TicketID), Initial: initialState, Final: finalState}
		if s := strings.TrimSpace(*flags.ArtifactDir); s != "" {
			out.ArtifactDir = s
		}
		if mrURL != "" {
			out.MRURL = mrURL
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
	}
	return nil
}

func defaultStageDefinitions() []modsapi.StageDefinition {
	return []modsapi.StageDefinition{
		{ID: domaintypes.StageID(modplan.StageNamePlan), Lane: "mods-plan", Priority: "default", MaxAttempts: 1},
		{ID: domaintypes.StageID(modplan.StageNameORWApply), Lane: "mods-java", Priority: "default", MaxAttempts: 1, Dependencies: []domaintypes.StageID{domaintypes.StageID(modplan.StageNamePlan)}},
		{ID: domaintypes.StageID(modplan.StageNameORWGenerate), Lane: "mods-java", Priority: "default", MaxAttempts: 1, Dependencies: []domaintypes.StageID{domaintypes.StageID(modplan.StageNamePlan)}},
		{ID: domaintypes.StageID(modplan.StageNameLLMPlan), Lane: "mods-llm", Priority: "default", MaxAttempts: 1, Dependencies: []domaintypes.StageID{domaintypes.StageID(modplan.StageNamePlan)}},
		{ID: domaintypes.StageID(modplan.StageNameLLMExec), Lane: "mods-llm", Priority: "default", MaxAttempts: 1, Dependencies: []domaintypes.StageID{domaintypes.StageID(modplan.StageNameORWApply), domaintypes.StageID(modplan.StageNameORWGenerate), domaintypes.StageID(modplan.StageNameLLMPlan)}},
	}
}

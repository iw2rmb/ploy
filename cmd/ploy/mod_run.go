package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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

func executeModRun(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	specFile := fs.String("spec", "", "Path to YAML/JSON spec file")
	repoURL := fs.String("repo-url", "", "Git repository URL to materialise for Mods execution")
	repoBaseRef := fs.String("repo-base-ref", "", "Git base ref used for materialisation")
	repoTargetRef := fs.String("repo-target-ref", "", "Git target ref created for the run")
	repoWorkspace := fs.String("repo-workspace-hint", "", "Optional subdirectory hint when preparing the workspace")
	follow := fs.Bool("follow", false, "follow ticket events until completion")
	capDuration := fs.Duration("cap", 0, "optional overall time cap for --follow (e.g., 5m)")
	cancelOnCap := fs.Bool("cancel-on-cap", false, "when set with --cap, cancel the ticket if the cap is exceeded")
	artifactDir := fs.String("artifact-dir", "", "directory to download final artifacts into (with manifest.json)")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON summary to stdout")
	maxRetries := fs.Int("max-retries", 5, "max reconnect attempts for event stream (-1 for unlimited)")
	retryWait := fs.Duration("retry-wait", 500*time.Millisecond, "wait between event stream reconnects")
	// Allow passing Mod env via repeated --mod-env KEY=VALUE
	var modEnvs stringSlice
	fs.Var(&modEnvs, "mod-env", "Mod environment KEY=VALUE (repeatable)")
	// Allow specifying the mod container image (paths fixed; image entrypoint runs)
	modImage := fs.String("mod-image", "", "Container image for the mod step (optional)")
	// Optional: retain container after run for inspection
	retain := fs.Bool("retain-container", false, "Retain the mod container after execution (for debugging)")
	// Optional: override container command (string, executed via sh -c on the node)
	modCommand := fs.String("mod-command", "", "Container command override (string or JSON array)")
	// GitLab MR flags (per-run overrides)
	gitlabPAT := fs.String("gitlab-pat", "", "GitLab Personal Access Token for this run (overrides server default)")
	gitlabDomain := fs.String("gitlab-domain", "", "GitLab domain for this run (overrides server default)")
	mrSuccess := fs.Bool("mr-success", false, "Create a merge request on success")
	mrFail := fs.Bool("mr-fail", false, "Create a merge request on failure")
	// DEPRECATED: --heal-on-build injects a default build_gate_healing when spec lacks it
	healOnBuild := fs.Bool("heal-on-build", false, "DEPRECATED: inject default build_gate_healing (use --spec with build_gate_healing instead)")

	if err := fs.Parse(args); err != nil {
		printModRunUsage(stderr)
		return err
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
		strings.TrimSpace(*specFile),
		modEnvs,
		strings.TrimSpace(*modImage),
		*retain,
		strings.TrimSpace(*modCommand),
		strings.TrimSpace(*gitlabPAT),
		strings.TrimSpace(*gitlabDomain),
		*mrSuccess,
		*mrFail,
		*healOnBuild,
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

	if *follow {
		followCtx := ctx
		var cancel context.CancelFunc
		if *capDuration > 0 {
			followCtx, cancel = context.WithTimeout(ctx, *capDuration)
			defer cancel()
		}
		ev := mods.EventsCommand{
			Client: stream.Client{
				HTTPClient:   cloneForStream(httpClient),
				MaxRetries:   *maxRetries,
				RetryBackoff: *retryWait,
			},
			BaseURL: base,
			Ticket:  string(summary.TicketID),
			Output:  stderr,
		}
		final, err := ev.Run(followCtx)
		if err != nil {
			if *capDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
				if *cancelOnCap {
					_, _ = fmt.Fprintln(stderr, "Follow timed out; requesting ticket cancellation...")
					_ = mods.CancelCommand{BaseURL: base, Client: httpClient, Ticket: string(summary.TicketID), Reason: "cap exceeded", Output: stderr}.Run(context.Background())
				} else {
					_, _ = fmt.Fprintf(stderr, "Follow capped after %s; ticket %s continues running in the background.\n", capDuration.String(), summary.TicketID)
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
		if strings.TrimSpace(*artifactDir) != "" {
			if err := downloadTicketArtifacts(ctx, base, httpClient, string(summary.TicketID), strings.TrimSpace(*artifactDir), stderr); err != nil {
				return err
			}
		}
	}

	if *jsonOut {
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
		if s := strings.TrimSpace(*artifactDir); s != "" {
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

func printModRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run [--spec <file>] [--repo-url <url> --repo-base-ref <branch> --repo-target-ref <branch> --repo-workspace-hint <dir>] [--mod-env KEY=VALUE ...] [--mod-image <image>] [--mod-command <cmd>] [--retain-container] [--gitlab-pat <token>] [--gitlab-domain <domain>] [--mr-success] [--mr-fail] [--heal-on-build (deprecated)] [--follow] [--cap <duration>] [--artifact-dir <dir>] [--json] [--max-retries N] [--retry-wait D]")
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

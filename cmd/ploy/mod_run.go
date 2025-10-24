package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/iw2rmb/ploy/internal/cli/mods"
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
	return nil
}

func printModRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run --tenant <tenant> [--ticket <ticket-id>|--ticket auto] [--repo-url <url> --repo-base-ref <branch> --repo-target-ref <branch> --repo-workspace-hint <dir>]")
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

package main

import (
	"context"
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

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleRunPatch implements:
//
//	ploy run patch [--repo-id <id> | --origin <remote>] [--diff-id <uuid>] [--output <path|->] <run-id>
//
// It is a read-only command: it downloads the stored patch artifact and does not apply it.
func handleRunPatch(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printRunPatchUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("run patch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	repoIDFlag := fs.String("repo-id", "", "repo id (skip repo-url resolution from git remote)")
	origin := fs.String("origin", "origin", "git remote to match when --repo-id is not provided")
	diffIDFlag := fs.String("diff-id", "", "specific diff id to download (default: latest)")
	output := fs.String("output", "-", "output path for .patch.gz bytes ('-' for stdout)")

	if err := parseFlagSet(fs, args, func() { printRunPatchUsage(stderr) }); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printRunPatchUsage(stderr)
		return errors.New("run-id required")
	}
	if len(rest) > 1 {
		printRunPatchUsage(stderr)
		return fmt.Errorf("unexpected argument: %s", rest[1])
	}

	runID := domaintypes.RunID(strings.TrimSpace(rest[0]))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("run patch: %w", err)
	}

	repoID, err := resolveRunPatchRepoID(ctx, httpClient, base, runID, strings.TrimSpace(*repoIDFlag), strings.TrimSpace(*origin))
	if err != nil {
		return fmt.Errorf("run patch: %w", err)
	}

	diffs, err := listRunRepoDiffs(ctx, httpClient, base, runID, repoID)
	if err != nil {
		return fmt.Errorf("run patch: list diffs: %w", err)
	}
	if len(diffs) == 0 {
		return errors.New("run patch: no diffs available for this repo execution")
	}

	selectedDiff, err := resolveRunPatchDiffID(diffs, strings.TrimSpace(*diffIDFlag))
	if err != nil {
		return fmt.Errorf("run patch: %w", err)
	}

	downloadCmd := migs.DownloadDiffGzipCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   runID,
		RepoID:  repoID,
		DiffID:  selectedDiff,
	}
	patchGzip, err := downloadCmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("run patch: download patch: %w", err)
	}

	if err := writeRunPatchOutput(strings.TrimSpace(*output), patchGzip); err != nil {
		return fmt.Errorf("run patch: write output: %w", err)
	}

	return nil
}

func resolveRunPatchRepoID(
	ctx context.Context,
	httpClient *http.Client,
	baseURL *url.URL,
	runID domaintypes.RunID,
	repoIDFlag string,
	origin string,
) (domaintypes.MigRepoID, error) {
	if repoIDFlag != "" {
		var repoID domaintypes.MigRepoID
		if err := repoID.UnmarshalText([]byte(repoIDFlag)); err != nil {
			return "", errors.New("invalid --repo-id")
		}
		return repoID, nil
	}

	rawOriginURL, err := resolveGitRemoteURL(ctx, origin)
	if err != nil {
		return "", err
	}

	pullCmd := migs.RunPullCommand{
		Client:  httpClient,
		BaseURL: baseURL,
		RunID:   runID,
		RepoURL: rawOriginURL,
	}
	resolution, err := pullCmd.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve repo: %w", err)
	}

	return resolution.RepoID, nil
}

func resolveRunPatchDiffID(diffs []migs.DiffEntry, diffIDFlag string) (domaintypes.DiffID, error) {
	if diffIDFlag == "" {
		// API ordering is ascending by execution chain / created_at; last is newest.
		return diffs[len(diffs)-1].ID, nil
	}

	var diffID domaintypes.DiffID
	if err := diffID.UnmarshalText([]byte(diffIDFlag)); err != nil {
		return "", errors.New("invalid --diff-id")
	}

	for _, item := range diffs {
		if item.ID == diffID {
			return diffID, nil
		}
	}
	return "", fmt.Errorf("diff %s not found in run repo diff listing", diffID)
}

func writeRunPatchOutput(outputPath string, patchGzip []byte) error {
	if outputPath == "" || outputPath == "-" {
		_, err := os.Stdout.Write(patchGzip)
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, patchGzip, 0o644)
}

func printRunPatchUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy run patch [--repo-id <id> | --origin <remote>] [--diff-id <uuid>] [--output <path|->] <run-id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Downloads a stored patch artifact (.patch.gz) without applying it.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --repo-id <id>       Repo ID (skip repo-url resolution from git remote)")
	_, _ = fmt.Fprintln(w, "  --origin <remote>    Git remote for repo-url resolution (default: origin)")
	_, _ = fmt.Fprintln(w, "  --diff-id <uuid>     Specific diff ID to download (default: latest)")
	_, _ = fmt.Fprintln(w, "  --output <path|->    Output path for raw .patch.gz bytes (default: -)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Notes:")
	_, _ = fmt.Fprintln(w, "  - This command does not run git prechecks.")
	_, _ = fmt.Fprintln(w, "  - This command does not create or switch branches.")
	_, _ = fmt.Fprintln(w, "  - This command does not apply patch content.")
}

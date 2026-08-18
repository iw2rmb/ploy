package run

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func finalizeRunSubmit(
	ctx context.Context,
	runID domaintypes.RunID,
	worktree string,
	specDisplayName string,
	out io.Writer,
	base *url.URL,
	httpClient *http.Client,
	opts SubmitOptions,
) error {
	final, err := followRunStatusReports(ctx, base, httpClient, runID, out, specDisplayName, opts.MaxRetries, time.Second)
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
		if err := DownloadRunArtifacts(ctx, base, httpClient, runID.String(), artifactDir, out); err != nil {
			return err
		}
	}

	if opts.Apply {
		return runApply(ctx, ApplyOptions{
			RunID:    runID.String(),
			RepoPath: worktree,
			Output:   out,
		}, base, httpClient)
	}
	return nil
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
		SpecDisplayName: renderOpts.SpecDisplayName,
		MaxRetries:      maxRetries,
		PollInterval:    pollInterval,
	}.Run(ctx)
}

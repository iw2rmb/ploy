package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/speccompiler"
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

	StepEnvOverrides map[string][]string
	BuildGateForced  buildGateForcedOverrides

	GitLabToken string

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
	if len(opts.StepEnvOverrides) > 0 {
		mutated, err := speccompiler.ApplyStepEnvOverrides(specPayload.Spec, opts.StepEnvOverrides)
		if err != nil {
			return err
		}
		specPayload.Spec = mutated
		specPayload.SpecID = ""
	}
	if opts.BuildGateForced.HasAny() {
		mutated, err := speccompiler.ApplyBuildGateForcedOverrides(specPayload.Spec, opts.BuildGateForced)
		if err != nil {
			return err
		}
		specPayload.Spec = mutated
		specPayload.SpecID = ""
	}

	repo, err := resolveSourceRepo(ctx, base, httpClient, opts.RepoSelector)
	if err != nil {
		return err
	}
	if opts.Apply && !repo.IsLocal {
		return errors.New("--apply requires a local repo")
	}

	var gitLabToken *string
	if strings.TrimSpace(opts.GitLabToken) != "" {
		gitLabToken = &opts.GitLabToken
	}

	request := domainapi.RunSubmitRequest{
		RepoURL:   domaintypes.RepoURL(repo.RepoURL),
		Ref:       domaintypes.GitRef(repo.Ref),
		CommitSHA: repo.CommitSHA,
		SpecID:    specPayload.SpecID,
		Spec:      specPayload.Spec,
		CreatedBy: strings.TrimSpace(os.Getenv("USER")),

		GitLabToken: gitLabToken,
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
	return finalizeRunSubmit(ctx, runID, repo.Worktree, specPayload.DisplayName, followOut, base, httpClient, opts)
}

package handlers

import (
	"context"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func resolveAndPersistSBOMStepSkip(
	ctx context.Context,
	st store.Store,
	job store.Job,
	mergedSpec []byte,
) (*contracts.SBOMStepSkipMetadata, error) {
	if domaintypes.JobType(job.JobType) != domaintypes.JobTypeSBOM {
		return nil, nil
	}

	pgStore, ok := st.(*store.PgStore)
	if !ok || pgStore == nil {
		return nil, nil
	}

	spec, err := contracts.ParseMigSpecJSON(mergedSpec)
	if err != nil {
		return nil, fmt.Errorf("parse merged spec for sbom cache: %w", err)
	}

	cycleCtx, ok := sbomCycleContextFromJob(job)
	if !ok {
		return nil, nil
	}
	lang, tool, release := sbomStackTupleFromSpec(spec, cycleCtx.Phase)
	if lang == "" || tool == "" || release == "" {
		return nil, nil
	}

	var refJobID *string
	var skip *contracts.SBOMStepSkipMetadata
	repoSHAIn := strings.TrimSpace(job.RepoShaIn)
	if sha40Pattern.MatchString(repoSHAIn) {
		row, err := pgStore.ResolveReusableSBOMByRepoSHAAndStack(ctx, store.ResolveReusableSBOMByRepoSHAAndStackParams{
			RepoID:    job.RepoID,
			RepoShaIn: repoSHAIn,
			Lang:      lang,
			Tool:      tool,
			Release:   release,
		})
		if err == nil {
			ref := row.RefJobID.String()
			refJobID = &ref
			if row.RefArtifactID != "" && strings.TrimSpace(row.RefJobImage) != "" {
				skip = &contracts.SBOMStepSkipMetadata{
					Enabled:       true,
					RefJobID:      row.RefJobID,
					RefArtifactID: strings.TrimSpace(row.RefArtifactID),
					RefJobImage:   strings.TrimSpace(row.RefJobImage),
				}
			}
		} else if !isNoRowsError(err) {
			return nil, fmt.Errorf("resolve reusable sbom: %w", err)
		}
	}

	if err := pgStore.UpsertSBOMStep(ctx, store.UpsertSBOMStepParams{
		JobID:    job.ID.String(),
		Lang:     lang,
		Tool:     tool,
		Release:  release,
		RefJobID: refJobID,
	}); err != nil {
		return nil, fmt.Errorf("upsert sbom cache row: %w", err)
	}

	return skip, nil
}

func sbomStackTupleFromSpec(spec *contracts.MigSpec, phase string) (lang, tool, release string) {
	if spec == nil || spec.BuildGate == nil {
		return "", "", ""
	}
	var cfg *contracts.BuildGatePhaseConfig
	if strings.TrimSpace(phase) == contracts.SBOMPhasePre {
		cfg = spec.BuildGate.Pre
	} else {
		cfg = spec.BuildGate.Post
	}
	if cfg == nil || cfg.Stack == nil || !cfg.Stack.Enabled {
		return "", "", ""
	}
	lang = strings.ToLower(strings.TrimSpace(cfg.Stack.Language))
	tool = strings.ToLower(strings.TrimSpace(cfg.Stack.Tool))
	release = strings.TrimSpace(cfg.Stack.Release)
	return lang, tool, release
}

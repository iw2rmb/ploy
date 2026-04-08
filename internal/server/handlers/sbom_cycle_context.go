package handlers

import (
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type sbomCycleContext struct {
	Phase     string
	Role      string
	RootJobID domaintypes.JobID
}

func sbomCycleContextFromJob(job store.Job) (sbomCycleContext, bool) {
	if len(job.Meta) > 0 {
		if meta, err := contracts.UnmarshalJobMeta(job.Meta); err == nil && meta.SBOM != nil {
			ctx := sbomCycleContext{
				Phase: strings.TrimSpace(meta.SBOM.Phase),
				Role:  strings.TrimSpace(meta.SBOM.Role),
			}
			if root := strings.TrimSpace(meta.SBOM.RootJobID); root != "" {
				ctx.RootJobID = domaintypes.JobID(root)
			}
			return normalizeSBOMCycleContext(ctx, job), true
		}
	}
	return inferLegacySBOMCycleContext(job)
}

func inferLegacySBOMCycleContext(job store.Job) (sbomCycleContext, bool) {
	name := strings.TrimSpace(job.Name)
	ctx := sbomCycleContext{Role: contracts.SBOMRoleInitial, RootJobID: job.ID}
	switch {
	case strings.HasPrefix(name, "pre-gate-"):
		ctx.Phase = contracts.SBOMPhasePre
	case strings.HasPrefix(name, "post-gate-"), strings.HasPrefix(name, "re-gate-"):
		ctx.Phase = contracts.SBOMPhasePost
	default:
		return sbomCycleContext{}, false
	}
	if strings.Contains(name, "-final-") || strings.HasSuffix(name, "-final-sbom") {
		ctx.Role = contracts.SBOMRoleFinal
	}
	return normalizeSBOMCycleContext(ctx, job), true
}

func normalizeSBOMCycleContext(ctx sbomCycleContext, job store.Job) sbomCycleContext {
	if strings.TrimSpace(ctx.Phase) != contracts.SBOMPhasePre && strings.TrimSpace(ctx.Phase) != contracts.SBOMPhasePost {
		ctx.Phase = contracts.SBOMPhasePost
	}
	if strings.TrimSpace(ctx.Role) == "" {
		ctx.Role = contracts.SBOMRoleInitial
	}
	if strings.TrimSpace(ctx.RootJobID.String()) == "" {
		ctx.RootJobID = job.ID
	}
	return ctx
}

func sbomCycleNameFromContext(ctx sbomCycleContext) string {
	if strings.TrimSpace(ctx.Phase) == contracts.SBOMPhasePre {
		return "pre-gate"
	}
	return "post-gate"
}

func sbomCycleContextMeta(ctx sbomCycleContext) *contracts.SBOMJobMetadata {
	return &contracts.SBOMJobMetadata{
		Phase:     strings.TrimSpace(ctx.Phase),
		Role:      strings.TrimSpace(ctx.Role),
		RootJobID: strings.TrimSpace(ctx.RootJobID.String()),
	}
}

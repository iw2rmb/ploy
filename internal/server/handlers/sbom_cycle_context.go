package handlers

import (
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type sbomCycleContext struct {
	Phase     string
	CycleName string
	Role      string
	RootJobID domaintypes.JobID
}

func sbomCycleContextFromJob(job store.Job) (sbomCycleContext, bool) {
	if len(job.Meta) > 0 {
		if meta, err := contracts.UnmarshalJobMeta(job.Meta); err == nil && meta.SBOM != nil {
			ctx := sbomCycleContext{
				Phase:     strings.TrimSpace(meta.SBOM.Phase),
				CycleName: strings.TrimSpace(meta.SBOM.CycleName),
				Role:      strings.TrimSpace(meta.SBOM.Role),
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
		ctx.CycleName = "pre-gate"
	case strings.HasPrefix(name, "post-gate-"), strings.HasPrefix(name, "re-gate-"):
		ctx.Phase = contracts.SBOMPhasePost
		if strings.HasPrefix(name, "re-gate-") {
			ctx.CycleName = cycleNameFromHookOrSBOMJobName(name)
		} else {
			ctx.CycleName = "post-gate"
		}
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
	if strings.TrimSpace(ctx.CycleName) == "" {
		if strings.TrimSpace(ctx.Phase) == contracts.SBOMPhasePre {
			ctx.CycleName = "pre-gate"
		} else {
			ctx.CycleName = "post-gate"
		}
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
	if cycleName := strings.TrimSpace(ctx.CycleName); cycleName != "" {
		return cycleName
	}
	if strings.TrimSpace(ctx.Phase) == contracts.SBOMPhasePre {
		return "pre-gate"
	}
	return "post-gate"
}

func sbomCycleContextMeta(ctx sbomCycleContext) *contracts.SBOMJobMetadata {
	return &contracts.SBOMJobMetadata{
		Phase:     strings.TrimSpace(ctx.Phase),
		CycleName: strings.TrimSpace(ctx.CycleName),
		Role:      strings.TrimSpace(ctx.Role),
		RootJobID: strings.TrimSpace(ctx.RootJobID.String()),
	}
}

func cycleNameFromHookOrSBOMJobName(name string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		return ""
	}
	if idx := strings.LastIndex(base, "-hook-"); idx > 0 {
		return strings.TrimSpace(base[:idx])
	}
	if idx := strings.LastIndex(base, "-sbom-"); idx > 0 {
		return strings.TrimSpace(base[:idx])
	}
	return ""
}

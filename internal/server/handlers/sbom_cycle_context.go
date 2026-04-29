package handlers

import (
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type sbomCycleContext struct {
	CycleName string
	RootJobID domaintypes.JobID
}

func sbomCycleContextFromJob(job store.Job) (sbomCycleContext, bool) {
	name := strings.TrimSpace(job.Name)
	ctx := sbomCycleContext{RootJobID: job.ID}
	switch {
	case strings.HasPrefix(name, "pre-gate-"):
		ctx.CycleName = "pre-gate"
	case strings.HasPrefix(name, "post-gate-"):
		ctx.CycleName = "post-gate"
	default:
		return sbomCycleContext{}, false
	}
	return ctx, true
}

func sbomCycleNameFromContext(ctx sbomCycleContext) string {
	if cycleName := strings.TrimSpace(ctx.CycleName); cycleName != "" {
		return cycleName
	}
	return "post-gate"
}

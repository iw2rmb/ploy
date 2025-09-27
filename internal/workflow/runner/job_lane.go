package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/lanes"
)

type laneDescriber interface {
	Describe(name string, opts lanes.DescribeOptions) (lanes.Description, error)
}

// LaneJobComposer composes job specifications using lane defaults.
type LaneJobComposer struct {
	Lanes laneDescriber
}

// Compose builds the job specification for the provided stage using lane metadata.
func (c LaneJobComposer) Compose(ctx context.Context, req JobComposeRequest) (StageJobSpec, error) {
	_ = ctx
	if c.Lanes == nil {
		return StageJobSpec{}, fmt.Errorf("lane registry unavailable")
	}
	lane := strings.TrimSpace(req.Stage.Lane)
	if lane == "" {
		return StageJobSpec{}, fmt.Errorf("lane is required for job composition")
	}
	manifest := req.Stage.Constraints.Manifest.Manifest
	desc, err := c.Lanes.Describe(lane, lanes.DescribeOptions{
		ManifestVersion: manifest.Version,
		AsterToggles:    req.Stage.Aster.Toggles,
	})
	if err != nil {
		return StageJobSpec{}, err
	}
	job := desc.Lane.Job
	env := make(map[string]string, len(job.Env))
	for key, value := range job.Env {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		env[trimmedKey] = strings.TrimSpace(value)
	}
	metadata := map[string]string{}
	if priority := strings.TrimSpace(job.Priority); priority != "" {
		metadata["priority"] = priority
	}
	return StageJobSpec{
		Image:   job.Image,
		Command: append([]string(nil), job.Command...),
		Env:     env,
		Resources: StageJobResources{
			CPU:    job.Resources.CPU,
			Memory: job.Resources.Memory,
			Disk:   job.Resources.Disk,
			GPU:    job.Resources.GPU,
		},
		Metadata: metadata,
	}, nil
}

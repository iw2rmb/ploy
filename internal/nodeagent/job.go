package nodeagent

import (
	"context"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// --- Job image name persistence ---

// SaveJobImageName persists the resolved container image name for a job to the control plane.
func (r *runController) SaveJobImageName(ctx context.Context, jobID types.JobID, image string) error {
	if r.jobImageNameSaver == nil {
		return fmt.Errorf("job image name saver not initialized")
	}
	image = strings.TrimSpace(image)
	if image == "" {
		return fmt.Errorf("image is empty")
	}
	return r.jobImageNameSaver.SaveJobImageName(ctx, jobID, image)
}

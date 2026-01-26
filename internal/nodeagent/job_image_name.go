package nodeagent

import (
	"context"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// SaveJobImageName persists the resolved container image name for a job to the control plane.
// This must be called before job execution starts so jobs.mod_image reflects the runtime image.
func (r *runController) SaveJobImageName(ctx context.Context, jobID types.JobID, image string) error {
	if err := r.ensureUploaders(); err != nil {
		return err
	}
	if r.jobImageNameSaver == nil {
		return fmt.Errorf("job image name saver not initialized")
	}
	image = strings.TrimSpace(image)
	if image == "" {
		return fmt.Errorf("image is empty")
	}
	return r.jobImageNameSaver.SaveJobImageName(ctx, jobID, image)
}

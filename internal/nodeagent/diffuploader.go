package nodeagent

import (
	"context"
	"fmt"
	"net/http"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// DiffUploader uploads diff and summary data to the control-plane server.
type DiffUploader struct {
	*baseUploader
}

// NewDiffUploader creates a new diff uploader.
func NewDiffUploader(cfg Config) (*DiffUploader, error) {
	base, err := newBaseUploader(cfg)
	if err != nil {
		return nil, err
	}
	return &DiffUploader{baseUploader: base}, nil
}

// UploadDiff compresses and uploads a diff to the server.
func (u *DiffUploader) UploadDiff(ctx context.Context, runID types.RunID, jobID types.JobID, diffBytes []byte, summary types.DiffSummary) error {
	gzippedDiff, err := gzipCompress(diffBytes, "gzipped diff")
	if err != nil {
		return err
	}
	payload := map[string]any{
		"patch":   gzippedDiff,
		"summary": summary,
	}
	apiPath := fmt.Sprintf("/v1/runs/%s/jobs/%s/diff", runID.String(), jobID.String())
	resp, err := u.postJSON(ctx, apiPath, payload, http.StatusCreated, "upload diff")
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

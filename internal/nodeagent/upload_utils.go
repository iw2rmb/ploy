package nodeagent

import (
	"context"
	"fmt"
)

// uploadOutDirIfPresent uploads the /out directory as an artifact bundle named "mod-out" when it contains files.
func uploadOutDirIfPresent(ctx context.Context, cfg Config, runID, stageID, outDir string) error {
	if outDir == "" {
		return nil
	}
	if hasFiles, files := listFilesRecursive(outDir); hasFiles {
		artifactUploader, err := NewArtifactUploader(cfg)
		if err != nil {
			return fmt.Errorf("create artifact uploader: %w", err)
		}
		if err := artifactUploader.UploadArtifact(ctx, runID, stageID, files, "mod-out"); err != nil {
			return fmt.Errorf("upload /out bundle: %w", err)
		}
	}
	return nil
}

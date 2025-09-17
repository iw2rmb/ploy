package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// uploadArtifactsAndMetadata uploads artifact bundle, SBOMs and metadata via unified or legacy clients
func uploadArtifactsAndMetadata(ctx context.Context, deps *BuildDependencies, srcDir, appName, sha, lane, imagePath, dockerImage string, sbom, signed bool) error {
	keyPrefix := appName + "/" + sha + "/"
	if deps.Storage != nil {
		if imagePath != "" {
			if err := uploadArtifactBundleWithUnifiedStorage(ctx, deps.Storage, keyPrefix, imagePath); err != nil {
				return fmt.Errorf("artifact bundle upload with verification failed: %w", err)
			}
		}
		// source SBOM
		sourceSBOMPath := filepath.Join(srcDir, ".sbom.json")
		if _, err := os.Stat(sourceSBOMPath); err == nil {
			_ = uploadFileWithUnifiedStorage(ctx, deps.Storage, sourceSBOMPath, keyPrefix+"source.sbom.json", "application/json")
		}
		// container SBOM
		if dockerImage != "" {
			containerSBOMPath := fmt.Sprintf("/tmp/%s-%s.sbom.json", appName, strings.ReplaceAll(dockerImage, "/", "-"))
			if _, err := os.Stat(containerSBOMPath); err == nil {
				_ = uploadFileWithUnifiedStorage(ctx, deps.Storage, containerSBOMPath, keyPrefix+"container.sbom.json", "application/json")
			}
		}
		meta := map[string]string{
			"lane": lane, "image": imagePath, "dockerImage": dockerImage,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"sbom":      fmt.Sprintf("%t", sbom), "signed": fmt.Sprintf("%t", signed),
		}
		mb, _ := json.Marshal(meta)
		_ = uploadBytesWithUnifiedStorage(ctx, deps.Storage, mb, keyPrefix+"meta.json", "application/json")
		return nil
	}

	// Legacy storage client
	if deps.StorageClient != nil {
		if imagePath != "" {
			if result, err := deps.StorageClient.UploadArtifactBundleWithVerification(keyPrefix, imagePath); err != nil {
				return fmt.Errorf("artifact bundle upload with verification failed: %w", err)
			} else if result != nil && !result.Verified {
				return fmt.Errorf("artifact integrity verification failed: %s", strings.Join(result.Errors, "; "))
			}
		}
		sourceSBOMPath := filepath.Join(srcDir, ".sbom.json")
		if _, err := os.Stat(sourceSBOMPath); err == nil {
			_ = uploadFileWithRetryAndVerification(deps.StorageClient, sourceSBOMPath, keyPrefix+"source.sbom.json", "application/json")
		}
		if dockerImage != "" {
			containerSBOMPath := fmt.Sprintf("/tmp/%s-%s.sbom.json", appName, strings.ReplaceAll(dockerImage, "/", "-"))
			if _, err := os.Stat(containerSBOMPath); err == nil {
				_ = uploadFileWithRetryAndVerification(deps.StorageClient, containerSBOMPath, keyPrefix+"container.sbom.json", "application/json")
			}
		}
		meta := map[string]string{
			"lane": lane, "image": imagePath, "dockerImage": dockerImage,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"sbom":      fmt.Sprintf("%t", sbom), "signed": fmt.Sprintf("%t", signed),
		}
		mb, _ := json.Marshal(meta)
		_ = uploadBytesWithRetryAndVerification(deps.StorageClient, mb, keyPrefix+"meta.json", "application/json")
		return nil
	}
	return nil
}

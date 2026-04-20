package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func (r *runController) materializeMigInFromInputs(
	ctx context.Context,
	req StartRunRequest,
	inDir string,
) error {
	if req.JobType != types.JobTypeMig || req.MigContext == nil || len(req.MigContext.InFrom) == 0 {
		return nil
	}
	if strings.TrimSpace(inDir) == "" {
		return fmt.Errorf("cross-step /in directory is required")
	}
	if r.artifactUploader == nil {
		return fmt.Errorf("artifact uploader is required for in_from inputs")
	}
	if err := os.MkdirAll(inDir, 0o755); err != nil {
		return fmt.Errorf("mkdir /in directory: %w", err)
	}

	cleanInDir := filepath.Clean(inDir)
	artifactCache := make(map[string][]byte, len(req.MigContext.InFrom))
	for i := range req.MigContext.InFrom {
		ref := req.MigContext.InFrom[i]
		sourceOutPath := strings.TrimSpace(ref.SourceOutPath)
		if sourceOutPath == "" {
			parsed, err := contracts.ParseInFromURI(ref.From)
			if err != nil {
				return fmt.Errorf("mig_context.in_from[%d].from: %w", i, err)
			}
			sourceOutPath = parsed.OutPath
		}
		targetPath, err := contracts.NormalizeInFromTarget(ref.To, sourceOutPath)
		if err != nil {
			return fmt.Errorf("mig_context.in_from[%d].to: %w", i, err)
		}
		rel := strings.TrimPrefix(targetPath, "/in/")
		destPath := filepath.Clean(filepath.Join(inDir, filepath.FromSlash(rel)))
		if destPath != cleanInDir && !strings.HasPrefix(destPath, cleanInDir+string(filepath.Separator)) {
			return fmt.Errorf("mig_context.in_from[%d].to: resolved path %s escapes /in", i, destPath)
		}

		artifactID := strings.TrimSpace(ref.SourceArtifactID)
		if artifactID == "" {
			return fmt.Errorf("mig_context.in_from[%d].source_artifact_id: required", i)
		}
		bundle, ok := artifactCache[artifactID]
		if !ok {
			downloaded, err := r.artifactUploader.DownloadArtifactBundle(ctx, artifactID)
			if err != nil {
				return fmt.Errorf("download artifact %q for mig_context.in_from[%d]: %w", artifactID, i, err)
			}
			bundle = downloaded
			artifactCache[artifactID] = bundle
		}

		payload, err := extractRegularFileFromArtifactOutPath(bundle, sourceOutPath)
		if err != nil {
			return fmt.Errorf("materialize mig_context.in_from[%d] from %q: %w", i, sourceOutPath, err)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("mkdir in_from destination dir: %w", err)
		}
		if err := os.WriteFile(destPath, payload, 0o644); err != nil {
			return fmt.Errorf("write in_from destination %s: %w", destPath, err)
		}
	}

	return nil
}

func extractRegularFileFromArtifactOutPath(bundle []byte, sourceOutPath string) ([]byte, error) {
	normalizedOutPath := path.Clean(strings.TrimSpace(sourceOutPath))
	if !strings.HasPrefix(normalizedOutPath, "/out/") || normalizedOutPath == "/out" {
		return nil, fmt.Errorf("source path must stay under /out")
	}
	targetEntry := strings.TrimPrefix(normalizedOutPath, "/")

	gzReader, err := gzip.NewReader(bytes.NewReader(bundle))
	if err != nil {
		return nil, fmt.Errorf("open artifact gzip: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("artifact bundle has no %s entry", targetEntry)
			}
			return nil, fmt.Errorf("read artifact tar header: %w", err)
		}
		if header == nil {
			continue
		}
		entry := normalizeBundlePath(header.Name)
		if entry != targetEntry {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("artifact entry %s is not a regular file", targetEntry)
		}
		payload, readErr := io.ReadAll(tarReader)
		if readErr != nil {
			return nil, fmt.Errorf("read artifact entry %s: %w", targetEntry, readErr)
		}
		return payload, nil
	}
}

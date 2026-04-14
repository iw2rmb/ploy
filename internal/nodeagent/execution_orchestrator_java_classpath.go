package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func runJavaClasspathPath(runID types.RunID) string {
	return filepath.Join(runCacheDir(runID), sbomJavaClasspathFileName)
}

func requiresJavaClasspath(req StartRunRequest) bool {
	if req.JobType == types.JobTypeSBOM || req.JavaClasspathContext == nil {
		return false
	}
	return req.JavaClasspathContext.Required
}

func (r *runController) materializeJavaClasspathInDir(
	ctx context.Context,
	req StartRunRequest,
	inDir string,
) error {
	if !requiresJavaClasspath(req) {
		return nil
	}
	if strings.TrimSpace(inDir) == "" {
		return fmt.Errorf("java classpath /in directory is required")
	}
	if err := os.MkdirAll(inDir, 0o755); err != nil {
		return fmt.Errorf("mkdir java classpath in dir: %w", err)
	}

	sourcePath := runJavaClasspathPath(req.RunID)
	if err := r.ensureRunJavaClasspathSource(ctx, req, sourcePath); err != nil {
		return err
	}

	destPath := filepath.Join(inDir, sbomJavaClasspathFileName)
	if err := copyFileBytes(sourcePath, destPath); err != nil {
		return fmt.Errorf("copy java classpath to /in: %w", err)
	}
	if err := validateJavaClasspathPath(destPath); err != nil {
		return fmt.Errorf("validate /in/%s: %w", sbomJavaClasspathFileName, err)
	}
	return nil
}

func (r *runController) ensureRunJavaClasspathSource(
	ctx context.Context,
	req StartRunRequest,
	sourcePath string,
) error {
	if err := validateJavaClasspathPath(sourcePath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("validate cached java classpath: %w", err)
	}

	artifactID := ""
	if req.JavaClasspathContext != nil {
		artifactID = strings.TrimSpace(req.JavaClasspathContext.SourceArtifactID)
	}
	if artifactID == "" {
		return fmt.Errorf("java classpath source artifact id is empty")
	}
	if r.artifactUploader == nil {
		return fmt.Errorf("artifact uploader is required to restore java classpath")
	}

	bundle, err := r.artifactUploader.DownloadArtifactBundle(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("download java classpath artifact %q: %w", artifactID, err)
	}
	if err := restoreJavaClasspathFromBundle(bundle, sourcePath); err != nil {
		return fmt.Errorf("restore java classpath from artifact %q: %w", artifactID, err)
	}
	if err := validateJavaClasspathPath(sourcePath); err != nil {
		return fmt.Errorf("validate restored java classpath: %w", err)
	}
	slog.Info("restored java classpath from artifact",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"artifact_id", artifactID,
		"path", sourcePath,
	)
	return nil
}

func restoreJavaClasspathFromBundle(bundle []byte, destPath string) error {
	gzReader, err := gzip.NewReader(bytes.NewReader(bundle))
	if err != nil {
		return fmt.Errorf("open artifact gzip: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)
	found := false
	for {
		header, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("read artifact tar header: %w", err)
		}
		if header == nil || header.Typeflag != tar.TypeReg {
			continue
		}
		entry := normalizeBundlePath(header.Name)
		if entry != "out/"+sbomJavaClasspathFileName {
			continue
		}
		payload, readErr := io.ReadAll(tarReader)
		if readErr != nil {
			return fmt.Errorf("read java classpath artifact entry: %w", readErr)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("mkdir java classpath cache dir: %w", err)
		}
		if err := os.WriteFile(destPath, payload, 0o644); err != nil {
			return fmt.Errorf("write java classpath cache file: %w", err)
		}
		found = true
		break
	}
	if !found {
		return fmt.Errorf("artifact bundle has no out/%s entry", sbomJavaClasspathFileName)
	}
	return nil
}

func persistRunJavaClasspath(runID types.RunID, srcPath string) error {
	if err := validateJavaClasspathPath(srcPath); err != nil {
		return err
	}
	destPath := runJavaClasspathPath(runID)
	if err := copyFileBytes(srcPath, destPath); err != nil {
		return err
	}
	return validateJavaClasspathPath(destPath)
}

func captureJavaClasspathForOutBundle(inDir, outDir string) error {
	if strings.TrimSpace(outDir) == "" {
		return nil
	}
	srcPath := filepath.Join(inDir, sbomJavaClasspathFileName)
	dstPath := filepath.Join(outDir, sbomJavaClasspathFileName)
	if err := copyFileBytes(srcPath, dstPath); err != nil {
		return err
	}
	return validateJavaClasspathPath(dstPath)
}

func (r *runController) captureJavaClasspathAfterStandardJob(req StartRunRequest, inDir, outDir string) error {
	if !requiresJavaClasspath(req) {
		return nil
	}
	if strings.TrimSpace(inDir) == "" {
		return fmt.Errorf("java classpath /in directory is not configured")
	}
	inClasspathPath := filepath.Join(inDir, sbomJavaClasspathFileName)
	if err := persistRunJavaClasspath(req.RunID, inClasspathPath); err != nil {
		return fmt.Errorf("persist run java classpath: %w", err)
	}
	if err := captureJavaClasspathForOutBundle(inDir, outDir); err != nil {
		return fmt.Errorf("seed /out/%s from /in: %w", sbomJavaClasspathFileName, err)
	}
	return nil
}

func (r *runController) prepareGateJavaClasspathInput(ctx context.Context, req StartRunRequest, workspace string) error {
	if !requiresJavaClasspath(req) {
		return nil
	}
	inDir := filepath.Join(workspace, step.BuildGateWorkspaceInDir)
	return r.materializeJavaClasspathInDir(ctx, req, inDir)
}

func (r *runController) captureJavaClasspathAfterGateJob(req StartRunRequest, workspace string) error {
	if !requiresJavaClasspath(req) {
		return nil
	}
	inDir := filepath.Join(workspace, step.BuildGateWorkspaceInDir)
	inClasspathPath := filepath.Join(inDir, sbomJavaClasspathFileName)
	if err := persistRunJavaClasspath(req.RunID, inClasspathPath); err != nil {
		return fmt.Errorf("persist run java classpath from gate /in: %w", err)
	}
	gateOutDir := filepath.Join(workspace, step.BuildGateWorkspaceOutDir)
	if err := captureJavaClasspathForOutBundle(inDir, gateOutDir); err != nil {
		return fmt.Errorf("seed gate /out/%s from /in: %w", sbomJavaClasspathFileName, err)
	}
	return nil
}

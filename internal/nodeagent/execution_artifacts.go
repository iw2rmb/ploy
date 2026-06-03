package nodeagent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

type artifactLogWriter struct {
	live      io.Writer
	stdout    *os.File
	stderr    *os.File
	closeOnce sync.Once
	closeErr  error
}

func newArtifactLogWriter(live io.Writer, paths jobArtifactPaths) (*artifactLogWriter, error) {
	stdout, err := os.OpenFile(paths.Stdout, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open stdout log: %w", err)
	}
	stderr, err := os.OpenFile(paths.Stderr, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = stdout.Close()
		return nil, fmt.Errorf("open stderr log: %w", err)
	}
	return &artifactLogWriter{live: live, stdout: stdout, stderr: stderr}, nil
}

func (w *artifactLogWriter) Write(p []byte) (int, error) {
	return w.StdoutWriter().Write(p)
}

func (w *artifactLogWriter) StdoutWriter() io.Writer {
	return w.streamWriter(w.stdout, true)
}

func (w *artifactLogWriter) StderrWriter() io.Writer {
	return w.streamWriter(w.stderr, false)
}

func (w *artifactLogWriter) streamWriter(file *os.File, stdout bool) io.Writer {
	writers := []io.Writer{file}
	if split, ok := w.live.(interface {
		StdoutWriter() io.Writer
		StderrWriter() io.Writer
	}); ok {
		if stdout {
			writers = append(writers, split.StdoutWriter())
		} else {
			writers = append(writers, split.StderrWriter())
		}
	} else if w.live != nil {
		writers = append(writers, w.live)
	}
	return io.MultiWriter(writers...)
}

func (w *artifactLogWriter) Close() error {
	w.closeOnce.Do(func() {
		if w.stdout != nil {
			if err := w.stdout.Close(); err != nil && w.closeErr == nil {
				w.closeErr = err
			}
		}
		if w.stderr != nil {
			if err := w.stderr.Close(); err != nil && w.closeErr == nil {
				w.closeErr = err
			}
		}
	})
	return w.closeErr
}

func (r *runController) uploadRepoArtifactsIfPresent(runID types.RunID, repoID types.MigRepoID, jobID types.JobID) {
	if r.artifactUploader == nil {
		return
	}
	artifactsDir := artifactsDir(runID)
	if artifactsDir == "" {
		return
	}
	hasFiles, _ := listFilesRecursive(artifactsDir)
	if !hasFiles {
		return
	}
	entries := []ArtifactBundleEntry{{
		SourcePath:  artifactsDir,
		ArchivePath: "artifacts",
	}}
	if _, _, err := r.artifactUploader.UploadArtifactEntries(context.Background(), runID, jobID, entries, "repo-artifacts"); err != nil {
		slog.Warn("failed to upload repo artifacts", "run_id", runID, "repo_id", repoID, "job_id", jobID, "error", err)
	}
}

func persistContainerInspectArtifact(req StartRunRequest, paths jobArtifactPaths, result step.Result) {
	if len(result.ContainerInspectJSON) == 0 || paths.Root == "" {
		return
	}
	path := filepath.Join(paths.Root, "container.inspect.json")
	if err := os.WriteFile(path, result.ContainerInspectJSON, 0o600); err != nil {
		slog.Warn("failed to write container inspect artifact", "run_id", req.RunID, "job_id", req.JobID, "container_id", result.ContainerID, "error", err)
	}
}

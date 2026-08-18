package nodeagent

import (
	"context"
	"encoding/json"
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

func newArtifactLogWriter(live io.Writer, dirs JobDirectories) (*artifactLogWriter, error) {
	stdout, err := os.OpenFile(dirs.Stdout, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open stdout log: %w", err)
	}
	stderr, err := os.OpenFile(dirs.Stderr, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
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

func (r *runController) uploadRepoArtifactsIfPresent(runID types.RunID, repoID types.RepoID, jobID types.JobID) {
	if r.artifactUploader == nil {
		return
	}
	entries, hasFiles, err := repoArtifactBundleEntries(runID)
	if err != nil {
		slog.Warn("failed to select repo artifacts", "run_id", runID, "repo_id", repoID, "job_id", jobID, "error", err)
		return
	}
	if !hasFiles {
		return
	}
	if _, _, err := r.artifactUploader.UploadArtifactEntries(context.Background(), runID, jobID, entries, "repo-artifacts"); err != nil {
		slog.Warn("failed to upload repo artifacts", "run_id", runID, "repo_id", repoID, "job_id", jobID, "error", err)
	}
}

func repoArtifactBundleEntries(runID types.RunID) ([]ArtifactBundleEntry, bool, error) {
	jobEntries, err := os.ReadDir(jobsDir(runID))
	if err != nil && !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("read run jobs directory: %w", err)
	}

	entries := make([]ArtifactBundleEntry, 0, len(jobEntries)*6+1)
	hasFiles := false
	for _, entry := range jobEntries {
		if !entry.IsDir() {
			continue
		}
		var jobID types.JobID
		if err := jobID.UnmarshalText([]byte(entry.Name())); err != nil {
			continue
		}
		dirs := jobDirectories(runID, jobID)
		archiveRoot := filepath.ToSlash(filepath.Join("artifacts", jobID.String()))
		for _, item := range []struct {
			source string
			name   string
		}{
			{source: dirs.In, name: "in"},
			{source: dirs.Out, name: "out"},
			{source: dirs.Stdout, name: "stdout.log"},
			{source: dirs.Stderr, name: "stderr.log"},
			{source: dirs.Diff, name: "diff.patch"},
			{source: dirs.ContainerInspect, name: "container.inspect.json"},
		} {
			info, statErr := os.Lstat(item.source)
			if os.IsNotExist(statErr) {
				continue
			}
			if statErr != nil {
				return nil, false, fmt.Errorf("stat durable job artifact %s: %w", item.source, statErr)
			}
			entries = append(entries, ArtifactBundleEntry{
				SourcePath:  item.source,
				ArchivePath: filepath.ToSlash(filepath.Join(archiveRoot, item.name)),
			})
			if info.IsDir() {
				dirHasFiles, _ := listFilesRecursive(item.source)
				hasFiles = hasFiles || dirHasFiles
			} else {
				hasFiles = true
			}
		}
	}

	shareDir := runShareDir(runID)
	if info, statErr := os.Lstat(shareDir); statErr == nil {
		entries = append(entries, ArtifactBundleEntry{
			SourcePath:  shareDir,
			ArchivePath: "artifacts/shared",
		})
		if info.IsDir() {
			shareHasFiles, _ := listFilesRecursive(shareDir)
			hasFiles = hasFiles || shareHasFiles
		} else {
			hasFiles = true
		}
	} else if !os.IsNotExist(statErr) {
		return nil, false, fmt.Errorf("stat durable run share %s: %w", shareDir, statErr)
	}

	return entries, hasFiles, nil
}

func persistContainerInspectArtifact(req StartRunRequest, dirs JobDirectories, result step.Result) {
	if len(result.ContainerInspectJSON) == 0 || dirs.ContainerInspect == "" {
		return
	}
	inspectJSON, err := redactContainerInspectExecutionData(result.ContainerInspectJSON)
	if err != nil {
		slog.Warn("failed to redact container inspect artifact", "run_id", req.RunID, "job_id", req.JobID, "container_id", result.ContainerID, "error", err)
		return
	}
	if err := os.WriteFile(dirs.ContainerInspect, inspectJSON, 0o600); err != nil {
		slog.Warn("failed to write container inspect artifact", "run_id", req.RunID, "job_id", req.JobID, "container_id", result.ContainerID, "error", err)
	}
}

func redactContainerInspectExecutionData(raw []byte) ([]byte, error) {
	var inspect map[string]any
	if err := json.Unmarshal(raw, &inspect); err != nil {
		return nil, fmt.Errorf("decode container inspect JSON: %w", err)
	}

	// Docker inspect repeats the job environment and command. Both can contain
	// credentials, while mounts, state, image, and timing remain useful evidence.
	delete(inspect, "Args")
	if config, ok := inspect["Config"].(map[string]any); ok {
		delete(config, "Env")
		delete(config, "Cmd")
		delete(config, "Entrypoint")
	}

	redacted, err := json.Marshal(inspect)
	if err != nil {
		return nil, fmt.Errorf("encode redacted container inspect JSON: %w", err)
	}
	return redacted, nil
}

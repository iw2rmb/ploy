package nodeagent

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

type splitBufferWriter struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func (w *splitBufferWriter) Write(p []byte) (int, error) {
	return w.stdout.Write(p)
}

func (w *splitBufferWriter) StdoutWriter() io.Writer {
	return &w.stdout
}

func (w *splitBufferWriter) StderrWriter() io.Writer {
	return &w.stderr
}

func TestArtifactLogWriterWritesFilesAndLiveStreams(t *testing.T) {
	root := t.TempDir()
	paths := jobArtifactPaths{
		Stdout: filepath.Join(root, "stdout.log"),
		Stderr: filepath.Join(root, "stderr.log"),
	}
	live := &splitBufferWriter{}
	writer, err := newArtifactLogWriter(live, paths)
	if err != nil {
		t.Fatalf("newArtifactLogWriter() error = %v", err)
	}

	if _, err := writer.StdoutWriter().Write([]byte("out\n")); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if _, err := writer.StderrWriter().Write([]byte("err\n")); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if got := string(mustReadFile(t, paths.Stdout)); got != "out\n" {
		t.Fatalf("stdout.log = %q", got)
	}
	if got := string(mustReadFile(t, paths.Stderr)); got != "err\n" {
		t.Fatalf("stderr.log = %q", got)
	}
	if got := live.stdout.String(); got != "out\n" {
		t.Fatalf("live stdout = %q", got)
	}
	if got := live.stderr.String(); got != "err\n" {
		t.Fatalf("live stderr = %q", got)
	}
}

func TestUploadRepoArtifactsIfPresent(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	jobID := types.NewJobID()
	env := newUploadTestEnv(t, runID.String(), jobID.String())

	paths := artifactPaths(runID, jobID)
	if err := ensureJobArtifactDirs(paths); err != nil {
		t.Fatalf("ensureJobArtifactDirs() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.Out, "result.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}
	if err := os.WriteFile(paths.Stdout, []byte("log"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	tmpDir := filepath.Join(runDir(runID), "tmp", jobID.String())
	if err := os.MkdirAll(tmpDir, 0o777); err != nil {
		t.Fatalf("create tmp dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "tool.jar"), []byte("tmp"), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}

	env.Controller.uploadRepoArtifactsIfPresent(runID, repoID, jobID)

	assertUpload(t, env.Calls, true, "repo-artifacts", []string{
		"artifacts/" + jobID.String() + "/out/result.txt",
		"artifacts/" + jobID.String() + "/stdout.log",
	})
	entries := tarEntriesFromBundle(t, (*env.Calls)[0].Bundle)
	for name := range entries {
		if name == "tmp/"+jobID.String()+"/tool.jar" || name == "artifacts/"+jobID.String()+"/tmp/tool.jar" {
			t.Fatalf("tmp file leaked into repo-artifacts as %q", name)
		}
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

package nodeagent

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
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
	paths := JobDirectories{
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
	repoID := types.NewRepoID()
	jobID := types.NewJobID()
	previousJobID := types.NewJobID()
	env := newUploadTestEnv(t, runID.String(), jobID.String())

	paths := jobDirectories(runID, jobID)
	if err := ensureRunDirectories(runID); err != nil {
		t.Fatalf("ensureRunDirectories() error = %v", err)
	}
	if err := ensureJobDirectories(paths); err != nil {
		t.Fatalf("ensureJobDirectories() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.In, "request.txt"), []byte("in"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.Out, "result.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}
	if err := os.WriteFile(paths.Stdout, []byte("log"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(paths.Stderr, []byte("error log"), 0o644); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := os.WriteFile(paths.Diff, []byte("diff"), 0o644); err != nil {
		t.Fatalf("write diff: %v", err)
	}
	if err := os.WriteFile(paths.ContainerInspect, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write container inspect: %v", err)
	}
	for _, runtimeDir := range []string{paths.Cache, paths.Home, paths.Staging, paths.Tmp} {
		if err := os.WriteFile(filepath.Join(runtimeDir, "private.bin"), []byte("runtime"), 0o644); err != nil {
			t.Fatalf("write runtime file: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(runShareDir(runID), "java.classpath"), []byte("entry"), 0o644); err != nil {
		t.Fatalf("write shared artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runRuntimeShareDir(runID), "dependency.jar"), []byte("runtime share"), 0o644); err != nil {
		t.Fatalf("write runtime share: %v", err)
	}
	previousPaths := jobDirectories(runID, previousJobID)
	if err := ensureJobDirectories(previousPaths); err != nil {
		t.Fatalf("ensure previous job directories: %v", err)
	}
	if err := os.WriteFile(filepath.Join(previousPaths.Out, "previous.txt"), []byte("previous"), 0o644); err != nil {
		t.Fatalf("write previous job output: %v", err)
	}

	env.Controller.uploadRepoArtifactsIfPresent(runID, repoID, jobID)

	assertUpload(t, env.Calls, true, "repo-artifacts", []string{
		"artifacts/" + jobID.String() + "/in/request.txt",
		"artifacts/" + jobID.String() + "/out/result.txt",
		"artifacts/" + jobID.String() + "/stdout.log",
		"artifacts/" + jobID.String() + "/stderr.log",
		"artifacts/" + jobID.String() + "/diff.patch",
		"artifacts/" + jobID.String() + "/container.inspect.json",
		"artifacts/" + previousJobID.String() + "/out/previous.txt",
		"artifacts/shared/java.classpath",
	})
	entries := tarEntriesFromBundle(t, (*env.Calls)[0].Bundle)
	for name := range entries {
		for _, forbidden := range []string{"/cache/", "/home/", "/staging/", "/tmp/", "runtime-share"} {
			if strings.Contains("/"+name, forbidden) {
				t.Fatalf("runtime file leaked into repo-artifacts as %q", name)
			}
		}
	}
}

func TestPersistContainerInspectArtifactRedactsExecutionData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "container.inspect.json")
	raw := []byte(`{
  "Args":["--token","argument-secret"],
  "State":{"Status":"exited"},
  "Config":{
    "Env":["API_TOKEN=environment-secret","PLAIN=value"],
    "Cmd":["tool","--password=command-secret"],
    "Entrypoint":["/bin/sh","credential-secret"],
    "Image":"registry.example.test/runtime:latest"
  }
}`)

	persistContainerInspectArtifact(
		StartRunRequest{RunID: types.RunID("run-redact"), JobID: types.JobID("job-redact")},
		JobDirectories{ContainerInspect: path},
		step.Result{ContainerID: "container-redact", ContainerInspectJSON: raw},
	)

	got := mustReadFile(t, path)
	for _, secret := range []string{"argument-secret", "environment-secret", "command-secret", "credential-secret"} {
		if bytes.Contains(got, []byte(secret)) {
			t.Fatalf("container inspect artifact contains secret %q: %s", secret, got)
		}
	}
	for _, omitted := range []string{`"Args"`, `"Env"`, `"Cmd"`, `"Entrypoint"`} {
		if bytes.Contains(got, []byte(omitted)) {
			t.Fatalf("container inspect artifact contains omitted field %s: %s", omitted, got)
		}
	}
	if !bytes.Contains(got, []byte(`"Image":"registry.example.test/runtime:latest"`)) ||
		!bytes.Contains(got, []byte(`"Status":"exited"`)) {
		t.Fatalf("container inspect artifact lost diagnostic fields: %s", got)
	}
}

func TestShouldUploadRepoArtifactsAfterMigJob(t *testing.T) {
	nextID := types.NewJobID()
	tests := []struct {
		name    string
		req     StartRunRequest
		outcome migJobOutcome
		want    bool
	}{
		{
			name:    "failure uploads",
			outcome: migJobOutcome{result: step.Result{ExitCode: 1}},
			want:    true,
		},
		{
			name: "terminal mig with disabled build gate uploads",
			req: StartRunRequest{
				MigSpec: &contracts.MigSpec{BuildGate: &contracts.BuildGateConfig{Disabled: true}},
			},
			outcome: migJobOutcome{result: step.Result{ExitCode: 0}},
			want:    true,
		},
		{
			name: "non-terminal mig with disabled build gate waits for successor",
			req: StartRunRequest{
				NextID:  &nextID,
				MigSpec: &contracts.MigSpec{BuildGate: &contracts.BuildGateConfig{Disabled: true}},
			},
			outcome: migJobOutcome{result: step.Result{ExitCode: 0}},
			want:    false,
		},
		{
			name:    "terminal mig with enabled build gate does not upload",
			req:     StartRunRequest{},
			outcome: migJobOutcome{result: step.Result{ExitCode: 0}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUploadRepoArtifactsAfterMigJob(tt.req, tt.outcome); got != tt.want {
				t.Fatalf("shouldUploadRepoArtifactsAfterMigJob() = %v, want %v", got, tt.want)
			}
		})
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

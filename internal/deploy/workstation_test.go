package deploy

import (
    "bytes"
    "context"
    "errors"
    "io"
    "os"
    "path/filepath"
    "runtime"
    "testing"
)

type recordingRunner struct{ calls [][]string }

func (r *recordingRunner) Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error {
    entry := append([]string{command}, append([]string(nil), args...)...)
    r.calls = append(r.calls, entry)
    return nil
}

func TestInstallMacAndLinuxSystemCA(t *testing.T) {
    rr := &recordingRunner{}
    opts := configureWorkstationOptions{ClusterID: "c1", CAPath: "/tmp/ca.pem", GOOS: "darwin", Runner: rr, Stderr: &bytes.Buffer{}}
    if err := installWorkstationCA(context.Background(), opts); err != nil {
        t.Fatalf("installWorkstationCA darwin: %v", err)
    }
    if len(rr.calls) == 0 {
        t.Fatalf("expected runner to be invoked for mac install")
    }

    // Linux path with update-ca-certificates present.
    rr.calls = nil
    binDir := t.TempDir()
    tool := filepath.Join(binDir, "update-ca-certificates")
    if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
        t.Fatalf("write tool: %v", err)
    }
    oldPath := os.Getenv("PATH")
    t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)
    opts = configureWorkstationOptions{ClusterID: "c1", CAPath: "/tmp/ca.pem", GOOS: "linux", Runner: rr, Stderr: &bytes.Buffer{}}
    if err := installWorkstationCA(context.Background(), opts); err != nil {
        t.Fatalf("installWorkstationCA linux: %v", err)
    }
    // Expect at least two calls: install and update-ca-certificates
    if len(rr.calls) < 2 {
        t.Fatalf("expected linux install to invoke runner twice, got %d", len(rr.calls))
    }
}

func TestEnsureResolverRecordDarwin(t *testing.T) {
    if runtime.GOOS == "windows" {
        t.Skip("resolver flow not applicable on Windows test runners")
    }
    rr := &recordingRunner{}
    dir := t.TempDir()
    var stderr bytes.Buffer
    in := bytes.NewBufferString("y\n")
    opts := configureWorkstationOptions{GOOS: "darwin", ResolverDir: dir, BeaconIP: "127.0.0.1", Runner: rr, Stdin: in, Stderr: &stderr}
    if err := ensureResolverRecord(context.Background(), opts); err != nil {
        t.Fatalf("ensureResolverRecord: %v", err)
    }
    if len(rr.calls) < 2 {
        t.Fatalf("expected mkdir and install calls, got %d", len(rr.calls))
    }
    // Ensure last call targets our resolver path.
    want := filepath.Join(dir, "ploy")
    last := rr.calls[len(rr.calls)-1]
    found := false
    for _, a := range last {
        if a == want {
            found = true
            break
        }
    }
    if !found {
        t.Fatalf("expected install command to reference %s; calls=%v", want, rr.calls)
    }
}

type errorRunner struct{ recordingRunner }

func (e *errorRunner) Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error {
    // Return a generic error to exercise warning paths; continue execution.
    e.recordingRunner.Run(ctx, command, args, stdin, streams)
    return errors.New("some error")
}

func TestInstallMacSystemCA_Warnings(t *testing.T) {
    er := &errorRunner{}
    opts := configureWorkstationOptions{ClusterID: "c1", CAPath: "/tmp/ca.pem", GOOS: "darwin", Runner: er, Stderr: &bytes.Buffer{}}
    // Should proceed despite runner errors, emitting warnings.
    if err := installWorkstationCA(context.Background(), opts); err != nil {
        t.Fatalf("installMac warnings path should not error: %v", err)
    }
}

func TestEnsureResolverRecordShortCircuits(t *testing.T) {
    // Existing file => skip
    rr := &recordingRunner{}
    dir := t.TempDir()
    path := filepath.Join(dir, "ploy")
    if err := os.WriteFile(path, []byte("nameserver 127.0.0.1\n"), 0o644); err != nil {
        t.Fatalf("write resolver: %v", err)
    }
    if err := ensureResolverRecord(context.Background(), configureWorkstationOptions{GOOS: "darwin", ResolverDir: dir, Runner: rr, Stderr: &bytes.Buffer{}}); err != nil {
        t.Fatalf("ensureResolverRecord existing err=%v", err)
    }
    // No beacon IP => skip with message
    if err := ensureResolverRecord(context.Background(), configureWorkstationOptions{GOOS: "darwin", ResolverDir: t.TempDir(), Stderr: &bytes.Buffer{}}); err != nil {
        t.Fatalf("ensureResolverRecord no beacon err=%v", err)
    }
}

func TestConfigureWorkstationLinuxHappy(t *testing.T) {
    rr := &recordingRunner{}
    binDir := t.TempDir()
    tool := filepath.Join(binDir, "update-ca-certificates")
    if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
        t.Fatalf("write tool: %v", err)
    }
    oldPath := os.Getenv("PATH")
    t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)
    opts := configureWorkstationOptions{ClusterID: "c1", CAPath: "/tmp/ca.pem", GOOS: "linux", Runner: rr, Stderr: &bytes.Buffer{}}
    if err := configureWorkstation(context.Background(), opts); err != nil {
        t.Fatalf("configureWorkstation linux: %v", err)
    }
}

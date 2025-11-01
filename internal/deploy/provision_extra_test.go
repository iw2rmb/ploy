package deploy

import (
    "context"
    "errors"
    "io"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestProvisionHostHappyPath(t *testing.T) {
    // Prepare a temporary ployd binary.
    bin := filepath.Join(t.TempDir(), "ployd")
    if err := os.WriteFile(bin, []byte("bin"), 0o755); err != nil {
        t.Fatalf("write temp ployd: %v", err)
    }
    // Record commands without actually running anything.
    rr := &recordingRunner{}
    opts := ProvisionOptions{
        Host:            "203.0.113.10",
        Address:         "203.0.113.10",
        User:            "root",
        IdentityFile:    "/root/.ssh/id_rsa",
        PloydBinaryPath: bin,
        Runner:          rr,
        ServiceChecks:   []string{"ployd"},
    }
    if err := ProvisionHost(context.Background(), opts); err != nil {
        t.Fatalf("ProvisionHost error: %v", err)
    }
    if len(rr.calls) < 3 {
        t.Fatalf("expected at least 3 runner calls (scp + 2x ssh), got %d", len(rr.calls))
    }
}

type failingStatusRunner struct{ recordingRunner }

func (f *failingStatusRunner) Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error {
    f.recordingRunner.Run(ctx, command, args, stdin, streams)
    // Simulate failure on is-active check to exercise status fallback path.
    joined := strings.Join(args, " ")
    if strings.Contains(joined, "systemctl is-active --quiet") {
        return errors.New("inactive")
    }
    return nil
}

func TestProvisionHostServiceCheckFailureTriggersStatus(t *testing.T) {
    bin := filepath.Join(t.TempDir(), "ployd")
    if err := os.WriteFile(bin, []byte("bin"), 0o755); err != nil {
        t.Fatalf("write temp ployd: %v", err)
    }
    fr := &failingStatusRunner{}
    opts := ProvisionOptions{Host: "h", Address: "h", PloydBinaryPath: bin, Runner: fr, ServiceChecks: []string{"ployd"}}
    err := ProvisionHost(context.Background(), opts)
    if err == nil || !strings.Contains(err.Error(), "service not active") {
        t.Fatalf("expected service not active error, got %v", err)
    }
    // Ensure a status command was attempted.
    found := false
    for _, call := range fr.calls {
        if len(call) > 1 && call[0] == "ssh" && strings.Contains(strings.Join(call[1:], " "), "systemctl status") {
            found = true
            break
        }
    }
    if !found {
        t.Fatalf("expected fallback status call, got calls=%v", fr.calls)
    }
}

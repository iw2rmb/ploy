package deploy

import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestFormatHostAndSanitizeNodeID(t *testing.T) {
    cases := []struct{ in, want string }{
        {"", "127.0.0.1"},
        {"192.168.1.2:22", "192.168.1.2"},
        {"2001:db8::1", "[2001:db8::1]"},
        {"[2001:db8::1]:22", "[2001:db8::1]"},
        {"example.com", "example.com"},
    }
    for _, c := range cases {
        if got := formatHost(c.in); got != c.want {
            t.Fatalf("formatHost(%q)=%q want %q", c.in, got, c.want)
        }
    }

    if got := sanitizeNodeID("  NODE_01  "); got != "node-01" {
        t.Fatalf("sanitizeNodeID=%q", got)
    }
    if got := sanitizeNodeID("!!!"); got != "" {
        t.Fatalf("sanitizeNodeID invalid produced %q", got)
    }
}

func TestPromptYesNo(t *testing.T) {
    in := bytes.NewBufferString("yes\n")
    var out bytes.Buffer
    ok, err := promptYesNo(in, &out, "Are you sure? ")
    if err != nil || !ok || !strings.Contains(out.String(), "Are you sure?") {
        t.Fatalf("promptYesNo got ok=%v err=%v out=%q", ok, err, out.String())
    }
    in = bytes.NewBufferString("n\n")
    ok, err = promptYesNo(in, &out, "?")
    if err != nil || ok {
        t.Fatalf("promptYesNo expected false, got ok=%v err=%v", ok, err)
    }
    // nil reader => default false
    ok, err = promptYesNo(nil, &out, "?")
    if err != nil || ok {
        t.Fatalf("promptYesNo nil got ok=%v err=%v", ok, err)
    }
}

func TestWorkstationHelpers_NoOps(t *testing.T) {
    // Unsupported OS in installWorkstationCA should no-op.
    if err := installWorkstationCA(context.Background(), configureWorkstationOptions{GOOS: "windows"}); err != nil {
        t.Fatalf("installWorkstationCA unsupported err=%v", err)
    }
    // Non-darwin ensureResolverRecord no-ops.
    if err := ensureResolverRecord(context.Background(), configureWorkstationOptions{GOOS: "linux"}); err != nil {
        t.Fatalf("ensureResolverRecord linux err=%v", err)
    }
    // configureWorkstation requires CAPath.
    if err := configureWorkstation(context.Background(), configureWorkstationOptions{GOOS: "linux"}); err == nil {
        t.Fatalf("expected configureWorkstation to error without CAPath")
    }
}

func TestShellQuote(t *testing.T) {
    cases := map[string]string{
        "": "''",
        "plain": "'plain'",
        "a'b": `'a'"'"'b'`,
    }
    for in, want := range cases {
        if got := shellQuote(in); got != want {
            t.Fatalf("shellQuote(%q)=%q want %q", in, got, want)
        }
    }
}

func TestRenderBootstrapScriptMergesEnv(t *testing.T) {
    script := renderBootstrapScript(map[string]string{"FOO": "bar", "PLOY_BOOTSTRAP_VERSION": "x"})
    if !strings.Contains(script, "export FOO=\"bar\"") {
        t.Fatalf("expected FOO export in script: %q", script)
    }
    // Default export should remain present even if overridden in input.
    if !strings.Contains(script, "PLOY_BOOTSTRAP_VERSION") {
        t.Fatalf("expected bootstrap version export in script: %q", script)
    }
}

func TestInstallLinuxSystemCA_UpdateCaTrust(t *testing.T) {
    rr := &recordingRunner{}
    // No update-ca-certificates; provide update-ca-trust instead.
    binDir := t.TempDir()
    tool := filepath.Join(binDir, "update-ca-trust")
    if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
        t.Fatalf("write tool: %v", err)
    }
    oldPath := os.Getenv("PATH")
    t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)
    opts := configureWorkstationOptions{ClusterID: "c1", CAPath: "/tmp/ca.pem", GOOS: "linux", Runner: rr, Stderr: &bytes.Buffer{}}
    if err := installWorkstationCA(context.Background(), opts); err != nil {
        t.Fatalf("installWorkstationCA linux (update-ca-trust): %v", err)
    }
    if len(rr.calls) < 2 {
        t.Fatalf("expected linux install to invoke runner twice, got %d", len(rr.calls))
    }
}

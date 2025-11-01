package main

import (
    "bytes"
    "strings"
    "testing"
)

func TestHandleNodeRequiresSubcommand(t *testing.T) {
    buf := &bytes.Buffer{}
    err := handleNode(nil, buf)
    if err == nil {
        t.Fatalf("expected error for missing node subcommand")
    }
    out := buf.String()
    if !strings.Contains(out, "Usage: ploy node") {
        t.Fatalf("expected node usage output, got: %q", out)
    }
}

func TestHandleNodeAddRequiresClusterIDAndAddress(t *testing.T) {
    buf := &bytes.Buffer{}
    // No flags at all -> cluster-id required first
    err := handleNodeAdd(nil, buf)
    if err == nil {
        t.Fatalf("expected error when --cluster-id is missing")
    }
    if !strings.Contains(err.Error(), "cluster-id is required") {
        t.Fatalf("unexpected error: %v", err)
    }

    // Provide cluster-id but no address
    buf.Reset()
    err = handleNodeAdd([]string{"--cluster-id", "abc"}, buf)
    if err == nil {
        t.Fatalf("expected error when --address is missing")
    }
    if !strings.Contains(err.Error(), "address is required") {
        t.Fatalf("unexpected error: %v", err)
    }
}

func TestHandleNodeAddRejectsExtraArgs(t *testing.T) {
    buf := &bytes.Buffer{}
    err := handleNodeAdd([]string{"--cluster-id", "c1", "--address", "1.2.3.4", "extra"}, buf)
    if err == nil {
        t.Fatalf("expected error for unexpected args")
    }
    if !strings.Contains(err.Error(), "unexpected arguments:") {
        t.Fatalf("unexpected error: %v", err)
    }
}

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHandleServerRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleServer(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing server subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy server") {
		t.Fatalf("expected server usage output, got: %q", out)
	}
}

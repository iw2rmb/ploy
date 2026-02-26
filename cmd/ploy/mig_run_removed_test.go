package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestModRunPull_IsRejected(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := executeCmd([]string{"mig", "run", "pull", "run_123"}, &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mig run pull has been removed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigRunRequiresMigRef(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := executeCmd([]string{"mig", "run"}, &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mig id/name required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigRunRejectsSingleRepoFlags(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := executeCmd([]string{"mig", "run", "--repo-url", "https://example.com/repo.git"}, &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mig id/name required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

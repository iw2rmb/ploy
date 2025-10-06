package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExecuteManifestSchemaPrintsJSON(t *testing.T) {
	prevPath := manifestSchemaPath
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	manifestSchemaPath = filepath.Join(repoRoot, "docs", "schemas", "integration_manifest.schema.json")
	defer func() { manifestSchemaPath = prevPath }()

	buf := &bytes.Buffer{}
	err := execute([]string{"manifest", "schema"}, buf)
	if err != nil {
		t.Fatalf("expected schema command to succeed, got %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "\"title\"") {
		t.Fatalf("expected schema output to contain title field, got %q", output)
	}
	if !strings.Contains(output, "integration_manifest.schema.json") {
		t.Fatalf("expected schema output to reference schema file, got %q", output)
	}
	if !strings.Contains(output, "\"manifest_version\"") {
		t.Fatalf("expected schema output to include manifest_version, got %q", output)
	}
}

func TestExecuteRequiresCommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := execute(nil, buf)
	if err == nil {
		t.Fatal("expected error when no command provided")
	}
	if buf.Len() == 0 {
		t.Fatal("expected usage output")
	}
}

func TestExecuteUnknownCommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := execute([]string{"unknown"}, buf)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestHandleWorkflowRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleWorkflow(nil, buf)
	if err == nil {
		t.Fatal("expected error for missing subcommand")
	}
	if !strings.Contains(buf.String(), "Usage: ploy workflow") {
		t.Fatalf("expected workflow usage, got %q", buf.String())
	}
}

func TestHandleModRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleMod(nil, buf)
	if err == nil {
		t.Fatal("expected error for missing mod subcommand")
	}
	if !strings.Contains(buf.String(), "Usage: ploy mod") {
		t.Fatalf("expected mod usage, got %q", buf.String())
	}
}

func TestPrintHelpers(t *testing.T) {
	buf := &bytes.Buffer{}
	printUsage(buf)
	printWorkflowUsage(buf)
	printRunUsage(buf, "workflow run")
	printWorkflowCancelUsage(buf)
	reportError(errors.New("boom"), buf)
	output := buf.String()
	for _, fragment := range []string{"Usage: ploy workflow run", "Usage: ploy workflow cancel", "Usage: ploy workflow", "error: boom"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

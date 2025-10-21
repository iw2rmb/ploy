package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteHelpMatchesGolden(t *testing.T) {
	t.Helper()
	buf := &bytes.Buffer{}
	err := execute([]string{"help"}, buf)
	if err != nil {
		t.Fatalf("execute help: %v", err)
	}
	expect := loadGolden(t, "help.txt")
	if diff := diffStrings(expect, buf.String()); diff != "" {
		t.Fatalf("help output mismatch:\n%s", diff)
	}
	if strings.Contains(buf.String(), "Grid") {
		t.Fatalf("help output should not reference Grid: %q", buf.String())
	}
}

func TestExecuteHelpForModMatchesGolden(t *testing.T) {
	t.Helper()
	buf := &bytes.Buffer{}
	err := execute([]string{"help", "mod"}, buf)
	if err != nil {
		t.Fatalf("execute help mod: %v", err)
	}
	expect := loadGolden(t, "help_mod.txt")
	if diff := diffStrings(expect, buf.String()); diff != "" {
		t.Fatalf("help mod output mismatch:\n%s", diff)
	}
}

func TestExecuteRequiresCommandPrintsHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	err := execute(nil, buf)
	if err == nil {
		t.Fatal("expected error when no arguments provided")
	}
	if !strings.Contains(buf.String(), "Ploy CLI v2") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func TestExecuteUnknownCommandSuggestsHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	err := execute([]string{"unknown"}, buf)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
	if !strings.Contains(buf.String(), "help") {
		t.Fatalf("expected help hint in usage output, got %q", buf.String())
	}
}

func loadGolden(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(data)
}

func diffStrings(expect, actual string) string {
	if expect == actual {
		return ""
	}
	return "expected:\n" + expect + "\nactual:\n" + actual
}

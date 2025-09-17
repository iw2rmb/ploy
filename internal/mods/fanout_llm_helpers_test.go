package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractJavaPathsFromError(t *testing.T) {
	errStr := "error: cannot find symbol\n  symbol:   class Foo\n  location: package com.example\nsrc/main/java/com/example/Foo.java:10: error: ';' expected\n"
	paths := extractJavaPathsFromError(errStr, 5)
	if len(paths) == 0 || paths[0] != "src/main/java/com/example/Foo.java" {
		t.Fatalf("expected to extract Foo.java path, got %#v", paths)
	}
}

func TestParseClassNamesFromError(t *testing.T) {
	errStr := "symbol: class Bar\nclass Baz extends Bar {}\n"
	names := parseClassNamesFromError(errStr, 5)
	if len(names) == 0 {
		t.Fatalf("expected at least one class name, got none")
	}
}

func TestFindJavaFilesByBasename(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "src/main/java/app"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f := filepath.Join(repo, "src/main/java/app/Hello.java")
	if err := os.WriteFile(f, []byte("class Hello {}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := findJavaFilesByBasename(repo, []string{"Hello"}, 5)
	if len(out) != 1 || out[0] != "src/main/java/app/Hello.java" {
		t.Fatalf("unexpected result: %#v", out)
	}
}

func TestParseMCPFromInputsCoercesBudgetNumbers(t *testing.T) {
	var inputs map[string]any
	const raw = `{
	  "model": "anthropic.claude-3",
	  "context": ["src/main/java/App.java"],
	  "prompts": ["regenerate"],
	  "tools": [{
	    "name": "search",
	    "endpoint": "https://mcp.dev/search",
	    "config": {"allow": "src/**"}
	  }],
	  "budgets": {
	    "max_tokens": 4096,
	    "max_cost": 17,
	    "timeout": "12m"
	  }
	}`
	if err := json.Unmarshal([]byte(raw), &inputs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg, err := parseMCPFromInputs(inputs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Model != "anthropic.claude-3" {
		t.Fatalf("unexpected model: %+v", cfg)
	}
	if len(cfg.Tools) != 1 || cfg.Tools[0].Name != "search" {
		t.Fatalf("unexpected tools: %+v", cfg.Tools)
	}
	if cfg.Budgets.MaxTokens != 4096 {
		t.Fatalf("expected max_tokens=4096, got %d", cfg.Budgets.MaxTokens)
	}
	if cfg.Budgets.MaxCost != 17 {
		t.Fatalf("expected max_cost=17, got %d", cfg.Budgets.MaxCost)
	}
	if cfg.Budgets.Timeout != "12m" {
		t.Fatalf("expected timeout=12m, got %q", cfg.Budgets.Timeout)
	}
}

func TestLLMFetchDiffIfProd(t *testing.T) {
	t.Setenv("PLOY_CONTROLLER", "")
	dir := t.TempDir()
	rendered := filepath.Join(dir, "llm_exec.rendered.hcl")
	if err := os.WriteFile(rendered, []byte("job \"llm-exec\" {}"), 0644); err != nil {
		t.Fatalf("write rendered: %v", err)
	}
	cap := &captureReporter{}
	origHead := headURLFn
	origDownload := downloadToFileFn
	var headCalls int
	headURLFn = func(url string) bool {
		headCalls++
		return true
	}
	downloadToFileFn = func(url, dest string) error {
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		return os.WriteFile(dest, []byte("patch"), 0644)
	}
	defer func() {
		headURLFn = origHead
		downloadToFileFn = origDownload
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := llmFetchDiffIfProd(ctx, cap, "http://seaweed", "mod-abc", "branch-1", "step-1", rendered); err != nil {
		t.Fatalf("llmFetchDiffIfProd: %v", err)
	}
	diffPath := filepath.Join(filepath.Dir(rendered), "out", "diff.patch")
	b, err := os.ReadFile(diffPath)
	if err != nil || string(b) != "patch" {
		t.Fatalf("unexpected diff contents: %v %q", err, string(b))
	}
	if headCalls == 0 {
		t.Fatalf("expected headURLFn to be called")
	}
	var sawStart, sawSuccess bool
	for _, ev := range cap.events {
		if strings.Contains(ev.Message, "download start") {
			sawStart = true
		}
		if strings.Contains(ev.Message, "download succeeded") {
			sawSuccess = true
		}
	}
	if !sawStart || !sawSuccess {
		t.Fatalf("expected download start and success events, got %+v", cap.events)
	}
}

func TestLLMFetchDiffIfProdRequiresInfra(t *testing.T) {
	cap := &captureReporter{}
	ctx := context.Background()
	dir := t.TempDir()
	rendered := filepath.Join(dir, "llm_exec.rendered.hcl")
	if err := os.WriteFile(rendered, []byte("job"), 0644); err != nil {
		t.Fatalf("write rendered: %v", err)
	}
	if err := llmFetchDiffIfProd(ctx, cap, "", "mod-abc", "branch-1", "step-1", rendered); err == nil {
		t.Fatal("expected error when seaweed URL missing")
	}
	if err := llmFetchDiffIfProd(ctx, cap, "http://seaweed", "", "branch-1", "step-1", rendered); err == nil {
		t.Fatal("expected error when modID missing")
	}
}

func TestLLMFetchDiffIfProdDownloadError(t *testing.T) {
	t.Setenv("PLOY_CONTROLLER", "")
	dir := t.TempDir()
	rendered := filepath.Join(dir, "llm_exec.rendered.hcl")
	if err := os.WriteFile(rendered, []byte("job"), 0644); err != nil {
		t.Fatalf("write rendered: %v", err)
	}
	origHead := headURLFn
	origDownload := downloadToFileFn
	headURLFn = func(string) bool { return true }
	downloadToFileFn = func(string, string) error { return fmt.Errorf("boom") }
	defer func() {
		headURLFn = origHead
		downloadToFileFn = origDownload
	}()
	cap := &captureReporter{}
	ctx := context.Background()
	err := llmFetchDiffIfProd(ctx, cap, "http://seaweed", "mod-xyz", "branch-2", "step-2", rendered)
	if err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("expected download failure, got %v", err)
	}
	var sawError bool
	for _, ev := range cap.events {
		if strings.Contains(ev.Message, "download failed") {
			sawError = true
			break
		}
	}
	if !sawError {
		t.Fatalf("expected error event, got %+v", cap.events)
	}
}

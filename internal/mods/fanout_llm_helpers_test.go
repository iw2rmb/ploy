package mods

import (
	"os"
	"path/filepath"
	"testing"
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

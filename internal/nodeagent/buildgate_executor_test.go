package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// helper to create a tar.gz with regular files
func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// helper to create a plain tar with a single symlink
func makeTarWithSymlink(t *testing.T, linkName, target string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Typeflag: tar.TypeSymlink, Name: linkName, Linkname: target, Mode: 0o777}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return buf.Bytes()
}

func TestExtractArchive_TarGz_Normal(t *testing.T) {
	e := &BuildGateExecutor{}
	ws := t.TempDir()
	data := makeTarGz(t, map[string]string{
		"a/b.txt": "hello",
		"c.txt":   "world",
	})
	if err := e.extractArchive(context.Background(), data, ws); err != nil {
		t.Fatalf("extractArchive error: %v", err)
	}
	// Verify files exist with content
	b1, err := os.ReadFile(filepath.Join(ws, "a", "b.txt"))
	if err != nil {
		t.Fatalf("read a/b.txt: %v", err)
	}
	if string(b1) != "hello" {
		t.Fatalf("a/b.txt content = %q", string(b1))
	}
	b2, err := os.ReadFile(filepath.Join(ws, "c.txt"))
	if err != nil {
		t.Fatalf("read c.txt: %v", err)
	}
	if string(b2) != "world" {
		t.Fatalf("c.txt content = %q", string(b2))
	}
}

func TestExtractArchive_PathTraversal(t *testing.T) {
	e := &BuildGateExecutor{}
	ws := t.TempDir()
	// Craft a plain tar with a traversal entry
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Typeflag: tar.TypeReg, Name: "../../evil.txt", Mode: 0o644, Size: int64(len("x"))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	if err := e.extractArchive(context.Background(), buf.Bytes(), ws); err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
}

func TestExtractArchive_SymlinkSkipped(t *testing.T) {
	e := &BuildGateExecutor{}
	ws := t.TempDir()
	data := makeTarWithSymlink(t, "l", "target")
	if err := e.extractArchive(context.Background(), data, ws); err != nil {
		t.Fatalf("extractArchive error: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(ws, "l")); !os.IsNotExist(err) {
		t.Fatalf("expected no link to be created, got err=%v", err)
	}
}

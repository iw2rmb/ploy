package mods

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateTarFromDir_GoArchive(t *testing.T) {
	dir := t.TempDir()
	// Create sample files
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "Main.java"), []byte("class Main {}"), 0644); err != nil {
		t.Fatal(err)
	}

	tarPath := filepath.Join(dir, "input.tar")
	if err := createTarFromDir(dir, tarPath); err != nil {
		t.Fatalf("createTarFromDir: %v", err)
	}
	st, err := os.Stat(tarPath)
	if err != nil {
		t.Fatalf("stat tar: %v", err)
	}
	if st.Size() == 0 {
		t.Fatalf("tar is empty")
	}

	// Open and verify a couple of entries exist
	f, err := os.Open(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tr := tar.NewReader(f)
	found := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Name == "./README.md" {
			found["readme"] = true
		}
		if hdr.Name == "./src/Main.java" {
			found["main"] = true
		}
	}
	if !found["readme"] || !found["main"] {
		t.Fatalf("expected entries not found in tar: %v", found)
	}
}

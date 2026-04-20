package nodeagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractRegularFileFromArtifactOutPath(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "payload.json")
	if err := os.WriteFile(srcFile, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	bundle, err := createTarGzBundleFromEntries([]ArtifactBundleEntry{
		{
			SourcePath:  srcFile,
			ArchivePath: "out/dependency-usage.nofilter.json",
		},
	})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}

	payload, err := extractRegularFileFromArtifactOutPath(bundle, "/out/dependency-usage.nofilter.json")
	if err != nil {
		t.Fatalf("extractRegularFileFromArtifactOutPath() error = %v", err)
	}
	if got, want := string(payload), `{"ok":true}`; got != want {
		t.Fatalf("payload=%q, want %q", got, want)
	}
}

func TestExtractRegularFileFromArtifactOutPath_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "payload.json")
	if err := os.WriteFile(srcFile, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	bundle, err := createTarGzBundleFromEntries([]ArtifactBundleEntry{
		{
			SourcePath:  srcFile,
			ArchivePath: "out/other.json",
		},
	})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}

	_, err = extractRegularFileFromArtifactOutPath(bundle, "/out/missing.json")
	if err == nil {
		t.Fatal("expected error")
	}
}

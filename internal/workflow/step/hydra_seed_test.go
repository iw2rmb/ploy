package step

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestSeedDirsFromStagingOut(t *testing.T) {
	tests := []struct {
		name     string
		hash     string
		fileName string
		body     string
		outEntry string // value placed into manifest.Out (without hash prefix)
		wantRel  string // path under outDir where content should appear
	}{
		{
			name:     "nested target path",
			hash:     "abc0000",
			fileName: "ok.txt",
			body:     "data",
			outEntry: "/out/results",
			wantRel:  filepath.Join("results", "ok.txt"),
		},
		{
			name:     "flat target path",
			hash:     "abc0000",
			fileName: "result.txt",
			body:     "output",
			outEntry: "/out/data",
			wantRel:  filepath.Join("data", "result.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stagingDir := t.TempDir()
			outDir := t.TempDir()

			contentDir := filepath.Join(stagingDir, tt.hash, "content")
			if err := os.MkdirAll(contentDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(contentDir, tt.fileName), []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}

			manifest := contracts.StepManifest{Out: []string{tt.hash + ":" + tt.outEntry}}
			if err := seedDirsFromStaging(manifest, JobMounts{Staging: stagingDir, Out: outDir}); err != nil {
				t.Fatalf("seedDirsFromStaging error: %v", err)
			}

			got, err := os.ReadFile(filepath.Join(outDir, tt.wantRel))
			if err != nil {
				t.Fatalf("seeded content missing: %v", err)
			}
			if string(got) != tt.body {
				t.Errorf("content = %q, want %q", got, tt.body)
			}
		})
	}
}

func TestSeedDirsFromStagingIn(t *testing.T) {
	stagingDir := t.TempDir()
	inDir := t.TempDir()

	fileHash := "abc0001"
	dirHash := "abc0002"

	fileContentPath := filepath.Join(stagingDir, fileHash, "content")
	if err := os.MkdirAll(filepath.Dir(fileContentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileContentPath, []byte("yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fileContentPath, 0o751); err != nil {
		t.Fatal(err)
	}
	fileModTime := time.Unix(1_704_444_444, 0).UTC()
	if err := os.Chtimes(fileContentPath, fileModTime, fileModTime); err != nil {
		t.Fatal(err)
	}

	dirContentPath := filepath.Join(stagingDir, dirHash, "content")
	if err := os.MkdirAll(dirContentPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirContentPath, "route.yaml"), []byte("route"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := contracts.StepManifest{
		In: []string{
			fileHash + ":/in/amata.yaml",
			dirHash + ":/in/amata",
		},
	}

	if err := seedDirsFromStaging(manifest, JobMounts{Staging: stagingDir, In: inDir}); err != nil {
		t.Fatalf("seedDirsFromStaging error: %v", err)
	}

	fileData, err := os.ReadFile(filepath.Join(inDir, "amata.yaml"))
	if err != nil {
		t.Fatalf("seeded file content missing: %v", err)
	}
	if string(fileData) != "yaml" {
		t.Errorf("seeded file content = %q, want %q", fileData, "yaml")
	}
	fileInfo, err := os.Stat(filepath.Join(inDir, "amata.yaml"))
	if err != nil {
		t.Fatalf("stat seeded file: %v", err)
	}
	if got, want := fileInfo.Mode().Perm(), os.FileMode(0o751); got != want {
		t.Fatalf("seeded file mode = %o, want %o", got, want)
	}
	if !fileInfo.ModTime().UTC().Equal(fileModTime) {
		t.Fatalf("seeded file modtime = %s, want %s", fileInfo.ModTime().UTC(), fileModTime)
	}

	dirData, err := os.ReadFile(filepath.Join(inDir, "amata", "route.yaml"))
	if err != nil {
		t.Fatalf("seeded dir content missing: %v", err)
	}
	if string(dirData) != "route" {
		t.Errorf("seeded dir content = %q, want %q", dirData, "route")
	}
}

func TestSeedDirsFromStagingTmp(t *testing.T) {
	stagingDir := t.TempDir()
	tmpDir := t.TempDir()

	fileHash := "abc0003"
	contentPath := filepath.Join(stagingDir, fileHash, "content")
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contentPath, []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := contracts.StepManifest{
		Tmp: []string{fileHash + ":/tmp/ploy/lib/tool.jar"},
	}

	if err := seedDirsFromStaging(manifest, JobMounts{Staging: stagingDir, Tmp: tmpDir}); err != nil {
		t.Fatalf("seedDirsFromStaging error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmpDir, "ploy", "lib", "tool.jar"))
	if err != nil {
		t.Fatalf("seeded tmp content missing: %v", err)
	}
	if string(got) != "jar" {
		t.Errorf("seeded tmp content = %q, want %q", got, "jar")
	}
}

func TestSeedDirsFromStagingCopiesOnlyWritableHomeEntries(t *testing.T) {
	stagingDir := t.TempDir()
	homeDir := t.TempDir()
	for hash, body := range map[string]string{
		"abc0003": "writable",
		"abc0004": "read-only",
	} {
		contentPath := filepath.Join(stagingDir, hash, "content")
		if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(contentPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	manifest := contracts.StepManifest{Home: []string{
		"abc0003:.codex/auth.json",
		"abc0004:.codex/config.toml:ro",
	}}
	if err := seedDirsFromStaging(manifest, JobMounts{Staging: stagingDir, Home: homeDir}); err != nil {
		t.Fatalf("seedDirsFromStaging() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(homeDir, ".codex", "auth.json"))
	if err != nil {
		t.Fatalf("read writable home entry: %v", err)
	}
	if string(got) != "writable" {
		t.Fatalf("writable home entry = %q", got)
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("read-only home entry was copied: %v", err)
	}
}

func TestHydraSeedTargetRejectsUnsupportedKind(t *testing.T) {
	t.Parallel()
	if _, err := hydraSeedTarget(contracts.HydraFileKind("future"), JobMounts{}); err == nil {
		t.Fatal("hydraSeedTarget() error = nil, want unsupported kind")
	}
}

package step

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneGateCacheDirOldestFirst_RemovesUntilThresholdReached(t *testing.T) {
	cacheDir := t.TempDir()
	oldPath := filepath.Join(cacheDir, "old")
	newPath := filepath.Join(cacheDir, "new")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old cache entry: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("write new cache entry: %v", err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old entry: %v", err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes new entry: %v", err)
	}

	orig := gateCacheFreeBytes
	t.Cleanup(func() { gateCacheFreeBytes = orig })
	call := 0
	gateCacheFreeBytes = func(string) (int64, error) {
		call++
		if call < 2 {
			return buildGateCacheMinFreeBytes - 1, nil
		}
		return buildGateCacheMinFreeBytes, nil
	}

	if err := pruneGateCacheDirOldestFirst(cacheDir); err != nil {
		t.Fatalf("pruneGateCacheDirOldestFirst() error: %v", err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected oldest entry removed, stat err=%v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected newest entry to remain, stat err=%v", err)
	}
}

func TestPruneGateCacheDirOldestFirst_RemovesAllWhenStillLowSpace(t *testing.T) {
	cacheDir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(cacheDir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write cache entry %s: %v", name, err)
		}
	}

	orig := gateCacheFreeBytes
	t.Cleanup(func() { gateCacheFreeBytes = orig })
	gateCacheFreeBytes = func(string) (int64, error) {
		return buildGateCacheMinFreeBytes - 1, nil
	}

	if err := pruneGateCacheDirOldestFirst(cacheDir); err != nil {
		t.Fatalf("pruneGateCacheDirOldestFirst() error: %v", err)
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("readdir cache dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected cache dir to be exhausted, entries=%d", len(entries))
	}
}

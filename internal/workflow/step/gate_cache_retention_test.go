package step

import (
	"errors"
	"testing"
)

func TestEnsureGateCacheCapacity_ReturnsNilWhenEnoughSpace(t *testing.T) {
	cacheDir := t.TempDir()
	orig := gateCacheFreeBytes
	t.Cleanup(func() { gateCacheFreeBytes = orig })
	gateCacheFreeBytes = func(string) (int64, error) {
		return buildGateCacheMinFreeBytes, nil
	}

	if err := ensureGateCacheCapacity(cacheDir); err != nil {
		t.Fatalf("ensureGateCacheCapacity() error: %v", err)
	}
}

func TestEnsureGateCacheCapacity_ReturnsNotEnoughSpaceWhenBelowThreshold(t *testing.T) {
	cacheDir := t.TempDir()

	orig := gateCacheFreeBytes
	t.Cleanup(func() { gateCacheFreeBytes = orig })
	gateCacheFreeBytes = func(string) (int64, error) {
		return buildGateCacheMinFreeBytes - 1, nil
	}

	err := ensureGateCacheCapacity(cacheDir)
	if !errors.Is(err, ErrNotEnoughSpace) {
		t.Fatalf("ensureGateCacheCapacity() error = %v, want %v", err, ErrNotEnoughSpace)
	}
}

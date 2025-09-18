package build

import (
	"os"
	"testing"
)

func TestBuildLogsURL_DefaultBase(t *testing.T) {
	t.Cleanup(func() { _ = os.Unsetenv("PLOY_SEAWEEDFS_URL") })
	_ = os.Unsetenv("PLOY_SEAWEEDFS_URL")
	got := buildLogsURL("build-logs/sample.log")
	want := "http://seaweedfs-filer.storage.ploy.local:8888/artifacts/build-logs/sample.log"
	if got != want {
		t.Fatalf("buildLogsURL default mismatch: got %q want %q", got, want)
	}
}

func TestBuildLogsURL_CustomBase(t *testing.T) {
	t.Setenv("PLOY_SEAWEEDFS_URL", "https://example.com/storage")
	got := buildLogsURL("build-logs/sample.log")
	want := "https://example.com/storage/artifacts/build-logs/sample.log"
	if got != want {
		t.Fatalf("buildLogsURL custom mismatch: got %q want %q", got, want)
	}
}

func TestBuildLogsURL_WithCollectionInEnv(t *testing.T) {
	t.Setenv("PLOY_SEAWEEDFS_URL", "https://example.com/storage/artifacts")
	got := buildLogsURL("build-logs/sample.log")
	want := "https://example.com/storage/artifacts/build-logs/sample.log"
	if got != want {
		t.Fatalf("buildLogsURL preserved collection: got %q want %q", got, want)
	}
}

func TestBuildLogsURL_EmptyKey(t *testing.T) {
	if buildLogsURL("") != "" {
		t.Fatalf("expected empty URL for empty key")
	}
}

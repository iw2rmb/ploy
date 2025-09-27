package snapshots

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type recordingArtifactPublisher struct {
	payload []byte
	cid     string
	err     error
}

func (r *recordingArtifactPublisher) Publish(ctx context.Context, data []byte) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	r.payload = append([]byte(nil), data...)
	if r.cid == "" {
		r.cid = "cid-test"
	}
	return r.cid, nil
}

type recordingMetadataPublisher struct {
	metadata SnapshotMetadata
	calls    int
	err      error
}

func (r *recordingMetadataPublisher) Publish(ctx context.Context, meta SnapshotMetadata) error {
	r.calls++
	r.metadata = meta
	return r.err
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	writeFile(t, path, string(data))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func startsWith(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate go.mod from tests")
		}
		dir = parent
	}
}

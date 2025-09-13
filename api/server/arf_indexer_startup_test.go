package server

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type stubFetcher struct{ jar []byte }

func (s stubFetcher) Fetch(pack, version string) ([]byte, error) { return s.jar, nil }

func buildJarWithYAML() []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, _ := zw.Create("META-INF/rewrite/test.yml")
	_, _ = f.Write([]byte("type: specs.openrewrite.org/v1beta/recipe\nname: test\n"))
	_ = zw.Close()
	return buf.Bytes()
}

func TestServer_IndexesDefaultPacksOnStartup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	// Use memory provider so server can resolve unified storage without external deps
	if err := os.WriteFile(cfgPath, []byte("storage:\n  provider: memory\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	srv, err := NewServer(&ControllerConfig{
		StorageConfigPath: cfgPath,
		ArfDefaultPacks:   "rewrite-java:1.0.0",
		ArfFetcher:        stubFetcher{jar: buildJarWithYAML()},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// Obtain storage via factory and check snapshot presence
	st := srv.indexerStorage
	if st == nil {
		t.Fatalf("expected indexer storage to be set")
	}
	// wait briefly for async indexer to persist snapshot
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if rc, err := st.Get(context.Background(), "artifacts/openrewrite/catalog.json"); err == nil {
			_ = rc.Close()
			return
		} else {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
		}
	}
	t.Fatalf("expected catalog snapshot, got error: %v", lastErr)
}

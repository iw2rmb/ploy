package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	providers_memory "github.com/iw2rmb/ploy/internal/storage/providers/memory"
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

func TestIndexer_Refresh_PersistsSnapshot(t *testing.T) {
	mem := providers_memory.NewMemoryStorage(0)
	idx := NewIndexer(stubFetcher{jar: buildJarWithYAML()}, mem)
	entries, err := idx.Refresh(context.Background(), []PackSpec{{Name: "rewrite-java", Version: "1.0.0"}})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected entries, got %d", len(entries))
	}
	rc, err := mem.Get(context.Background(), catalogKey)
	if err != nil {
		t.Fatalf("snapshot not persisted: %v", err)
	}
	_ = rc.Close()
}

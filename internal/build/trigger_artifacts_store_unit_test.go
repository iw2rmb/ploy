package build

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	mem "github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

func TestUploadArtifactsAndMetadata_WithUnifiedStorage_MetaOnly(t *testing.T) {
	storage := mem.NewMemoryStorage(0)
	deps := &BuildDependencies{Storage: storage}

	ctx := context.Background()
	srcDir := t.TempDir()

	err := uploadArtifactsAndMetadata(ctx, deps, srcDir, "app", "sha", "E", "", "", false, false)
	require.NoError(t, err)

	exists, err := storage.Exists(ctx, "app/sha/meta.json")
	require.NoError(t, err)
	require.True(t, exists)
}

func TestUploadArtifactsAndMetadata_WithUnifiedStorage_ArtifactBundle(t *testing.T) {
	storage := mem.NewMemoryStorage(0)
	deps := &BuildDependencies{Storage: storage}

	ctx := context.Background()
	srcDir := t.TempDir()

	artifact := filepath.Join(srcDir, "image.bin")
	require.NoError(t, os.WriteFile(artifact, []byte("data"), 0644))
	require.NoError(t, os.WriteFile(artifact+".sig", []byte("sig"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, ".sbom.json"), []byte("{}"), 0644))

	err := uploadArtifactsAndMetadata(ctx, deps, srcDir, "app", "sha", "E", artifact, "", true, true)
	require.NoError(t, err)

	// main artifact
	r, err := storage.Get(ctx, "app/sha/"+filepath.Base(artifact))
	require.NoError(t, err)
	b, _ := io.ReadAll(r)
	_ = r.Close()
	require.Equal(t, []byte("data"), b)

	// source sbom
	ok, err := storage.Exists(ctx, "app/sha/source.sbom.json")
	require.NoError(t, err)
	require.True(t, ok)

	// signature optional
	ok, err = storage.Exists(ctx, "app/sha/"+filepath.Base(artifact)+".sig")
	require.NoError(t, err)
	require.True(t, ok)
}

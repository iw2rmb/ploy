package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsurePersistentArtifactCopy_CopiesArtifactAndSidecars(t *testing.T) {
	originalDir := persistentArtifactsDir
	destDir := t.TempDir()
	persistentArtifactsDir = destDir
	t.Cleanup(func() { persistentArtifactsDir = originalDir })

	srcDir := t.TempDir()
	artifact := filepath.Join(srcDir, "image.qcow2")
	require.NoError(t, os.WriteFile(artifact, []byte("image"), 0600))
	require.NoError(t, os.WriteFile(artifact+".sig", []byte("sig"), 0600))
	require.NoError(t, os.WriteFile(artifact+".sbom.json", []byte("sbom"), 0600))

	copyPath, err := ensurePersistentArtifactCopy(artifact)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(destDir, filepath.Base(artifact)), copyPath)

	for _, suffix := range []string{"", ".sig", ".sbom.json"} {
		_, statErr := os.Stat(copyPath + suffix)
		require.NoError(t, statErr)
	}
}

func TestEnsurePersistentArtifactCopy_NoSource(t *testing.T) {
	copyPath, err := ensurePersistentArtifactCopy("")
	require.NoError(t, err)
	require.Empty(t, copyPath)
}

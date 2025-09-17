package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindWASMArtifact(t *testing.T) {
	dir := t.TempDir()

	_, err := findWASMArtifact(dir)
	require.Error(t, err)

	p := filepath.Join(dir, "mod.wasm")
	require.NoError(t, os.WriteFile(p, []byte("wasm"), 0644))

	got, err := findWASMArtifact(dir)
	require.NoError(t, err)
	require.Equal(t, p, got)
}

func TestNewSandboxService(t *testing.T) {
	s := NewSandboxService()
	require.NotNil(t, s)
}

package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxRun_ErrorsOnEmptyRepo(t *testing.T) {
	s := NewSandboxService()
	_, err := s.Run(context.Background(), SandboxRequest{})
	require.Error(t, err)
}

func TestSandboxRun_LaneG_SucceedsWithWASM(t *testing.T) {
	dir := t.TempDir()
	wasm := filepath.Join(dir, "module.wasm")
	require.NoError(t, os.WriteFile(wasm, []byte("wasm"), 0644))

	s := NewSandboxService()
	res, err := s.Run(context.Background(), SandboxRequest{RepoPath: dir, AppName: "demo", Lane: "G"})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success)
	require.Equal(t, "G", res.BuildSystem)
	require.Len(t, res.Artifacts, 1)
	require.Equal(t, wasm, res.Artifacts[0].Path)
	require.Equal(t, "wasm", res.Artifacts[0].Type)
}

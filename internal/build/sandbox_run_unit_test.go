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

func TestSandboxRun_ForcesLaneD(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("demo"), 0o644))

	s := NewSandboxService()
	res, err := s.Run(context.Background(), SandboxRequest{RepoPath: dir, AppName: "demo", Lane: "G"})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success)
	require.Equal(t, "D", res.BuildSystem)
	require.Len(t, res.Artifacts, 1)
	require.Equal(t, "oci", res.Artifacts[0].Type)
}

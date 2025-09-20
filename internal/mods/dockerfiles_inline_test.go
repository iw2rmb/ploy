package mods

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDockerfileExtrasForLaneD_GeneratesPair(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644))

	extras, err := dockerfileExtrasForLaneD(dir)
	require.NoError(t, err)
	require.NotNil(t, extras)
	require.Contains(t, extras, "build.Dockerfile")
	require.Contains(t, extras, "deploy.Dockerfile")
	require.Contains(t, string(extras["build.Dockerfile"]), "FROM maven")
	require.Contains(t, string(extras["deploy.Dockerfile"]), "ENTRYPOINT")

	_, err = os.Stat(filepath.Join(dir, "build.Dockerfile"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "deploy.Dockerfile"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestDockerfileExtrasForLaneD_SkipsWhenExisting(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "build.Dockerfile"), []byte("FROM scratch"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deploy.Dockerfile"), []byte("FROM scratch"), 0o644))

	extras, err := dockerfileExtrasForLaneD(dir)
	require.NoError(t, err)
	require.Nil(t, extras)
}

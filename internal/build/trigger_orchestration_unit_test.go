package build

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldSkipLaneDeploy(t *testing.T) {
	t.Setenv("MODS_SKIP_DEPLOY_LANES", "A, g ,x")
	require.True(t, shouldSkipLaneDeploy("a"))
	require.True(t, shouldSkipLaneDeploy("G"))
	require.True(t, shouldSkipLaneDeploy("x"))
	require.False(t, shouldSkipLaneDeploy("b"))
}

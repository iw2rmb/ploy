package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildLaneF_CreatesVMImage(t *testing.T) {
	tmp := t.TempDir()
	img, err := buildLaneF("app", "sha", tmp, map[string]string{})
	require.NoError(t, err)
	require.NotEmpty(t, img)
	// Ensure vm.img exists
	p := filepath.Join(tmp, "vm.img")
	_, statErr := os.Stat(p)
	require.NoError(t, statErr)
}

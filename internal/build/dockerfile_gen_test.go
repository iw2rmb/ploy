package build

import (
	"os"
	"path/filepath"
	"testing"

	project "github.com/iw2rmb/ploy/internal/detect/project"
	"github.com/stretchr/testify/require"
)

func TestGenerateDockerfile_PythonAppPyOnly(t *testing.T) {
	dir := t.TempDir()
	// minimal python app marker
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("print('ok')"), 0644))

	err := generateDockerfileWithFacts(dir, project.BuildFacts{})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	s := string(b)
	require.Contains(t, s, "FROM python:")
	require.Contains(t, s, "CMD [\"python\", \"app.py\"]")
}

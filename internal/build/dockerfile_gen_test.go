package build

import (
	"os"
	"path/filepath"
	"strings"
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

func TestGenerateDockerfile_GradleMainClass(t *testing.T) {
	dir := t.TempDir()
	facts := project.BuildFacts{
		Language:  "java",
		BuildTool: "gradle",
		MainClass: "com.example.App",
		Versions:  project.Versions{Java: "17.0.2"},
	}

	require.NoError(t, generateDockerfileWithFacts(dir, facts))

	b, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	content := string(b)
	require.Contains(t, content, "FROM gradle:8-jdk17 AS build")
	require.Contains(t, content, "ENTRYPOINT")
	require.Contains(t, content, "com.example.App")
}

func TestGenerateDockerfile_NodeProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0644))

	require.NoError(t, generateDockerfileWithFacts(dir, project.BuildFacts{}))

	b, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	s := string(b)
	require.Contains(t, s, "FROM node:20-alpine")
	require.Contains(t, s, `CMD ["node", "index.js"]`)
}

func TestGenerateDockerfile_DotnetProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Demo.csproj"), []byte("<Project></Project>"), 0644))
	facts := project.BuildFacts{Language: "dotnet", Versions: project.Versions{Dotnet: "8.0.1"}}

	require.NoError(t, generateDockerfileWithFacts(dir, facts))

	b, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	s := string(b)
	require.Contains(t, s, "FROM mcr.microsoft.com/dotnet/sdk:8.0")
	require.Contains(t, s, "ENTRYPOINT [\"dotnet\", \"Demo.dll\"]")
}

func TestGenerateDockerfile_PythonGunicornDetection(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("gunicorn==21.2.0"), 0644))
	facts := project.BuildFacts{Language: "python", Versions: project.Versions{Python: "3.11.5"}}

	require.NoError(t, generateDockerfileWithFacts(dir, facts))

	b, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	s := string(b)
	require.Contains(t, s, "FROM python:3.11-slim")
	require.Contains(t, s, "gunicorn")
	require.True(t, strings.Contains(s, "exec gunicorn"))
}

func TestGenerateDockerfile_Unsupported(t *testing.T) {
	dir := t.TempDir()
	err := generateDockerfileWithFacts(dir, project.BuildFacts{Language: "rust"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported autogeneration")
}

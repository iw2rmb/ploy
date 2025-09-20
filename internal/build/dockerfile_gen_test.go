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

	s, err := generateDockerfileWithFacts(dir, project.BuildFacts{})
	require.NoError(t, err)
	require.NotEmpty(t, s)

	_, statErr := os.Stat(filepath.Join(dir, "Dockerfile"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	require.Contains(t, s, "FROM python:3.12-slim AS build")
	require.Contains(t, s, "COPY --from=build /out/app ./app")
	require.Contains(t, s, "CMD [\"python\", \"app/app.py\"]")
}

func TestGenerateDockerfile_GradleMainClass(t *testing.T) {
	dir := t.TempDir()
	facts := project.BuildFacts{
		Language:  "java",
		BuildTool: "gradle",
		MainClass: "com.example.App",
		Versions:  project.Versions{Java: "17.0.2"},
	}

	content, err := generateDockerfileWithFacts(dir, facts)
	require.NoError(t, err)
	require.NotEmpty(t, content)
	_, statErr := os.Stat(filepath.Join(dir, "Dockerfile"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	require.Contains(t, content, "FROM gradle:8-jdk17 AS build")
	require.Contains(t, content, "FROM eclipse-temurin:17-jre-alpine")
	require.Contains(t, content, "COPY --from=build /out/app.jar /app/app.jar")
	require.Contains(t, content, "ENTRYPOINT")
	require.Contains(t, content, "com.example.App")
}

func TestRenderDockerfilePair_Gradle(t *testing.T) {
	facts := project.BuildFacts{
		Language:  "java",
		BuildTool: "gradle",
		MainClass: "com.example.Main",
		Versions:  project.Versions{Java: "17"},
	}

	set, err := RenderDockerfilePair(facts)
	require.NoError(t, err)

	require.Contains(t, set.Build, "FROM gradle:8-jdk17")
	require.NotContains(t, set.Build, "FROM eclipse-temurin")

	require.Contains(t, set.Deploy, "FROM eclipse-temurin:17-jre-alpine")
	require.Contains(t, set.Deploy, "COPY app.jar /app/app.jar")
	require.NotContains(t, set.Deploy, "--from=build")
	require.Contains(t, set.Deploy, "com.example.Main")
}

func TestRenderDockerfilePair_Maven(t *testing.T) {
	facts := project.BuildFacts{
		Language:  "java",
		BuildTool: "maven",
		Versions:  project.Versions{Java: "11.0.20"},
	}

	pair, err := RenderDockerfilePair(facts)
	require.NoError(t, err)

	require.Contains(t, pair.Build, "FROM maven:3-eclipse-temurin-11")
	require.Contains(t, pair.Deploy, "FROM eclipse-temurin:11-jre-alpine")
	require.Contains(t, pair.Deploy, "COPY app.jar /app/app.jar")
}

func TestRenderDockerfilePair_Go(t *testing.T) {
	facts := project.BuildFacts{Language: "go", Versions: project.Versions{Go: "1.22"}}
	pair, err := RenderDockerfilePair(facts, WithCopyInstruction("COPY --from=build /out/app /app"))
	require.NoError(t, err)

	require.Contains(t, pair.Build, "FROM golang:1.22-alpine AS build")
	require.Contains(t, pair.Build, "CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./...")
	require.Contains(t, pair.Deploy, "FROM gcr.io/distroless/static")
	require.Contains(t, pair.Deploy, "COPY --from=build /out/app /app")
}

func TestRenderDockerfilePair_Unsupported(t *testing.T) {
	_, err := RenderDockerfilePair(project.BuildFacts{Language: "ruby"})
	require.Error(t, err)
}

func TestGenerateDockerfile_MavenLeanRuntime(t *testing.T) {
	dir := t.TempDir()
	facts := project.BuildFacts{
		Language:  "java",
		BuildTool: "maven",
		Versions:  project.Versions{Java: "11.0.20"},
	}

	content, err := generateDockerfileWithFacts(dir, facts)
	require.NoError(t, err)
	require.NotEmpty(t, content)
	_, statErr := os.Stat(filepath.Join(dir, "Dockerfile"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	require.Contains(t, content, "FROM maven:3-eclipse-temurin-11 AS build")
	require.Contains(t, content, "JAR=\"$(find target")
	require.Contains(t, content, "FROM eclipse-temurin:11-jre-alpine")
	require.Contains(t, content, "COPY --from=build /out/app.jar /app/app.jar")
}

func TestGenerateDockerfile_NodeProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0644))

	s, err := generateDockerfileWithFacts(dir, project.BuildFacts{})
	require.NoError(t, err)
	require.NotEmpty(t, s)
	_, statErr := os.Stat(filepath.Join(dir, "Dockerfile"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	require.Contains(t, s, "FROM node:20-alpine")
	require.Contains(t, s, `CMD ["node", "index.js"]`)
}

func TestGenerateDockerfile_GoProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.22\n"), 0644))

	content, err := generateDockerfileWithFacts(dir, project.BuildFacts{})
	require.NoError(t, err)
	require.NotEmpty(t, content)
	_, statErr := os.Stat(filepath.Join(dir, "Dockerfile"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	require.Contains(t, content, "FROM golang:1.22-alpine AS build")
	require.Contains(t, content, "CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./...")
	require.Contains(t, content, "FROM gcr.io/distroless/static")
	require.Contains(t, content, "COPY --from=build /out/app /app")
}

func TestGenerateDockerfile_DotnetProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Demo.csproj"), []byte("<Project></Project>"), 0644))
	facts := project.BuildFacts{Language: "dotnet", Versions: project.Versions{Dotnet: "8.0.1"}}

	s, err := generateDockerfileWithFacts(dir, facts)
	require.NoError(t, err)
	require.NotEmpty(t, s)
	_, statErr := os.Stat(filepath.Join(dir, "Dockerfile"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	require.Contains(t, s, "FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build")
	require.Contains(t, s, "FROM mcr.microsoft.com/dotnet/aspnet:8.0")
	require.Contains(t, s, "ENTRYPOINT [\"dotnet\", \"Demo.dll\"]")
}

func TestGenerateDockerfile_PythonGunicornDetection(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("gunicorn==21.2.0"), 0644))
	facts := project.BuildFacts{Language: "python", Versions: project.Versions{Python: "3.11.5"}}

	s, err := generateDockerfileWithFacts(dir, facts)
	require.NoError(t, err)
	require.NotEmpty(t, s)
	_, statErr := os.Stat(filepath.Join(dir, "Dockerfile"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	require.Contains(t, s, "FROM python:3.11-slim AS build")
	require.Contains(t, s, `["sh", "-lc", "exec gunicorn -b 0.0.0.0:$PORT app:app"]`)
}

func TestGenerateDockerfile_Unsupported(t *testing.T) {
	dir := t.TempDir()
	_, err := generateDockerfileWithFacts(dir, project.BuildFacts{Language: "rust"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported autogeneration")
}

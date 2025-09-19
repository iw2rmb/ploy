package build

import (
	"testing"

	project "github.com/iw2rmb/ploy/internal/detect/project"
	"github.com/stretchr/testify/require"
)

func TestRenderDockerfilePair_GoTemplates(t *testing.T) {
	facts := project.BuildFacts{Language: "go", Versions: project.Versions{Go: "1.22"}}
	pair, err := RenderDockerfilePair(facts, WithCopyInstruction("COPY --from=build /out/app /app"))
	require.NoError(t, err)

	require.Contains(t, pair.Build, "FROM golang:1.22-alpine AS build")
	require.Contains(t, pair.Build, "CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./...")
	require.Contains(t, pair.Deploy, "FROM gcr.io/distroless/static")
	require.Contains(t, pair.Deploy, "COPY --from=build /out/app /app")
}

func TestRenderDockerfilePair_NodeTemplates(t *testing.T) {
	facts := project.BuildFacts{Language: "node", Versions: project.Versions{Node: "20"}}
	pair, err := RenderDockerfilePair(facts, WithCopyInstruction("COPY --from=build /app /app"))
	require.NoError(t, err)

	require.Contains(t, pair.Build, "FROM node:20-alpine")
	require.Contains(t, pair.Build, "npm install --omit=dev")
	require.Contains(t, pair.Deploy, "FROM node:20-alpine")
	require.Contains(t, pair.Deploy, "COPY --from=build /app /app")
}

func TestRenderDockerfilePair_PythonTemplates(t *testing.T) {
	facts := project.BuildFacts{Language: "python", Versions: project.Versions{Python: "3.12"}}
	pair, err := RenderDockerfilePair(facts, WithPythonRuntimeCommands(`["sh", "-lc", "exec gunicorn"]`, ""))
	require.NoError(t, err)

	require.Contains(t, pair.Build, "FROM python:3.12-slim AS build")
	require.Contains(t, pair.Deploy, "FROM python:3.12-slim")
	require.Contains(t, pair.Deploy, `["sh", "-lc", "exec gunicorn"]`)
}

func TestRenderDockerfilePair_DotnetTemplates(t *testing.T) {
	facts := project.BuildFacts{Language: "dotnet", Versions: project.Versions{Dotnet: "8.0.1"}}
	pair, err := RenderDockerfilePair(facts, WithDotnetProjectName("Demo"))
	require.NoError(t, err)

	require.Contains(t, pair.Build, "FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build")
	require.Contains(t, pair.Deploy, "FROM mcr.microsoft.com/dotnet/aspnet:8.0")
	require.Contains(t, pair.Deploy, "ENTRYPOINT [\"dotnet\", \"Demo.dll\"]")
}

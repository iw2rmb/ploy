package mods

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureDockerfilePair_CreatesFiles(t *testing.T) {
	cases := []struct {
		name         string
		setup        func(string)
		expectBuild  string
		expectDeploy string
	}{
		{
			name: "gradle",
			setup: func(dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "build.gradle"), []byte("apply plugin: 'java'"), 0o644))
			},
			expectBuild:  "FROM gradle:8-jdk",
			expectDeploy: "FROM eclipse-temurin",
		},
		{
			name: "go",
			setup: func(dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.22\n"), 0o644))
			},
			expectBuild:  "FROM golang:1.22-alpine AS build",
			expectDeploy: "FROM gcr.io/distroless/static",
		},
		{
			name: "node",
			setup: func(dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0o644))
			},
			expectBuild:  "FROM node:20-alpine",
			expectDeploy: "FROM node:20-alpine",
		},
		{
			name: "python",
			setup: func(dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("gunicorn==21.2.0"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("print('ok')"), 0o644))
			},
			expectBuild:  "FROM python:3.12-slim AS build",
			expectDeploy: "exec gunicorn",
		},
		{
			name: "dotnet",
			setup: func(dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "Demo.csproj"), []byte("<Project></Project>"), 0o644))
			},
			expectBuild:  "FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build",
			expectDeploy: "ENTRYPOINT [\"dotnet\", \"Demo.dll\"]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(dir)

			require.NoError(t, ensureDockerfilePair(dir))

			buildBytes, err := os.ReadFile(filepath.Join(dir, "build.Dockerfile"))
			require.NoError(t, err)
			deployBytes, err := os.ReadFile(filepath.Join(dir, "deploy.Dockerfile"))
			require.NoError(t, err)

			require.Contains(t, string(buildBytes), tc.expectBuild)
			require.Contains(t, string(deployBytes), tc.expectDeploy)
		})
	}
}

package mods

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureDockerfilePair_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	gradleFile := filepath.Join(dir, "build.gradle")
	if err := os.WriteFile(gradleFile, []byte("apply plugin: 'java'"), 0o644); err != nil {
		t.Fatalf("write build.gradle: %v", err)
	}

	if err := ensureDockerfilePair(dir); err != nil {
		t.Fatalf("ensureDockerfilePair returned error: %v", err)
	}

	buildPath := filepath.Join(dir, "build.Dockerfile")
	deployPath := filepath.Join(dir, "deploy.Dockerfile")

	buildBytes, err := os.ReadFile(buildPath)
	if err != nil {
		t.Fatalf("read build.Dockerfile: %v", err)
	}
	deployBytes, err := os.ReadFile(deployPath)
	if err != nil {
		t.Fatalf("read deploy.Dockerfile: %v", err)
	}

	if !strings.Contains(string(buildBytes), "FROM gradle:8-jdk") {
		t.Fatalf("build.Dockerfile missing gradle base: %s", string(buildBytes))
	}
	if !strings.Contains(string(deployBytes), "FROM eclipse-temurin") {
		t.Fatalf("deploy.Dockerfile missing temurin base: %s", string(deployBytes))
	}
}

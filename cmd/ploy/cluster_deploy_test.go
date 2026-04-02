package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	iversion "github.com/iw2rmb/ploy/internal/version"
)

func TestParseClusterDeployArgs(t *testing.T) {
	got, err := parseClusterDeployArgs([]string{"--drop-db", "--nodes", "--no-pull", "--cluster", "demo"}, io.Discard)
	if err != nil {
		t.Fatalf("parseClusterDeployArgs returned error: %v", err)
	}
	want := []string{"--drop-db", "--nodes", "--no-pull", "--cluster", "demo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected forwarded args: got %v, want %v", got, want)
	}
}

func TestParseClusterDeployArgsRejectsClusterConflict(t *testing.T) {
	_, err := parseClusterDeployArgs([]string{"--cluster", "demo", "other"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "cluster specified both") {
		t.Fatalf("expected cluster conflict error, got %v", err)
	}
}

func TestParseClusterDeployArgsGeneratesClusterIDWhenUnset(t *testing.T) {
	oldGen := generateClusterDeployID
	defer func() { generateClusterDeployID = oldGen }()
	generateClusterDeployID = func() (string, error) { return "autogen-id-1234", nil }

	got, err := parseClusterDeployArgs(nil, io.Discard)
	if err != nil {
		t.Fatalf("parseClusterDeployArgs returned error: %v", err)
	}
	want := []string{"--cluster", "autogen-id-1234"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected forwarded args: got %v, want %v", got, want)
	}
}

func TestBuildClusterDeployEnvRequiresSemverWhenUnset(t *testing.T) {
	t.Setenv("PLOY_VERSION", "")
	oldVersion := iversion.Version
	iversion.Version = "dev"
	defer func() { iversion.Version = oldVersion }()

	_, err := buildClusterDeployEnv(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "PLOY_VERSION is required") {
		t.Fatalf("expected missing PLOY_VERSION error, got %v", err)
	}
}

func TestBuildClusterDeployEnvSetsComposeAndVersion(t *testing.T) {
	t.Setenv("PLOY_VERSION", "")
	t.Setenv("COMPOSE_CMD", "")
	oldVersion := iversion.Version
	iversion.Version = "v1.2.3"
	defer func() { iversion.Version = oldVersion }()

	deployDir := t.TempDir()
	env, err := buildClusterDeployEnv(deployDir)
	if err != nil {
		t.Fatalf("buildClusterDeployEnv returned error: %v", err)
	}

	expectCompose := "COMPOSE_CMD=docker compose -f " + filepath.Join(deployDir, "docker-compose.yml")
	expectVersion := "PLOY_VERSION=v1.2.3"
	if !containsExactEnvEntry(env, expectCompose) {
		t.Fatalf("expected env to contain %q", expectCompose)
	}
	if !containsExactEnvEntry(env, expectVersion) {
		t.Fatalf("expected env to contain %q", expectVersion)
	}
}

func TestHandleClusterDeployExecutesAndAlwaysCleansUp(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configHome)

	oldRunner := runClusterDeployScript
	defer func() { runClusterDeployScript = oldRunner }()

	var called bool
	runClusterDeployScript = func(ctx context.Context, scriptPath string, args []string, env []string, stdout, stderr io.Writer) error {
		called = true
		if !strings.HasSuffix(scriptPath, string(filepath.Separator)+"deploy"+string(filepath.Separator)+"run.sh") {
			t.Fatalf("unexpected script path: %s", scriptPath)
		}
		if _, err := os.Stat(scriptPath); err != nil {
			t.Fatalf("expected extracted run.sh to exist: %v", err)
		}
		wantArgs := []string{"--drop-db", "--cluster", "demo"}
		if !reflect.DeepEqual(args, wantArgs) {
			t.Fatalf("unexpected forwarded args: got %v, want %v", args, wantArgs)
		}
		return errors.New("runner failed")
	}

	err := handleClusterDeploy([]string{"--drop-db", "--cluster", "demo"}, bytes.NewBuffer(nil))
	if err == nil || !strings.Contains(err.Error(), "runner failed") {
		t.Fatalf("expected runner error, got %v", err)
	}
	if !called {
		t.Fatal("expected runtime deploy runner to be called")
	}

	deployPath := filepath.Join(configHome, "deploy")
	if _, statErr := os.Stat(deployPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected %s to be deleted, stat err=%v", deployPath, statErr)
	}
}

func containsExactEnvEntry(env []string, entry string) bool {
	for _, e := range env {
		if e == entry {
			return true
		}
	}
	return false
}

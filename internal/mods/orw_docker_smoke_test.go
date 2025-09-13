//go:build docker

package mods

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// This smoke test runs the real openrewrite-jvm container against a cloned repo to produce a diff.patch.
// It is opt-in and requires a local Docker daemon and a reachable SeaweedFS filer.
// To run:
//   - Ensure Docker is running and you have access to the specified image.
//   - Export: TRANSFLOW_DOCKER_SMOKE=1
//   - Optionally override:
//     ORW_IMAGE=registry.dev.ployman.app/openrewrite-jvm:latest
//     REPO_URL=https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
//     PLOY_SEAWEEDFS_URL=http://localhost:8888
//     RECIPE_CLASS, RECIPE_GROUP, RECIPE_ARTIFACT, RECIPE_VERSION, MAVEN_PLUGIN_VERSION
//   - Run: go test -tags=docker -run TestORWApplyDocker_Smoke ./internal/mods -v
func TestORWApplyDocker_Smoke(t *testing.T) {
	if os.Getenv("TRANSFLOW_DOCKER_SMOKE") != "1" {
		t.Skip("set TRANSFLOW_DOCKER_SMOKE=1 to enable this smoke test")
	}

	// Check docker binary
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available: ", err)
	}

	image := os.Getenv("ORW_IMAGE")
	if image == "" {
		image = "registry.dev.ployman.app/openrewrite-jvm:latest"
	}

	repoURL := os.Getenv("REPO_URL")
	if repoURL == "" {
		repoURL = "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git"
	}

	seaweed := os.Getenv("PLOY_SEAWEEDFS_URL")
	if seaweed == "" {
		t.Skip("PLOY_SEAWEEDFS_URL not set; required for artifact upload")
	}

	// Recipe coordinates (defaults from orw-apply-manual.sh)
	recipeClass := getenvDefault("RECIPE_CLASS", "org.openrewrite.java.migrate.UpgradeToJava17")
	recipeGroup := getenvDefault("RECIPE_GROUP", "org.openrewrite.recipe")
	recipeArtifact := getenvDefault("RECIPE_ARTIFACT", "rewrite-migrate-java")
	recipeVersion := getenvDefault("RECIPE_VERSION", "3.17.0")
	pluginVersion := getenvDefault("MAVEN_PLUGIN_VERSION", "6.18.0")

	work := t.TempDir()
	repoDir := filepath.Join(work, "repo")

	// Shallow clone
	if out, err := exec.Command("git", "clone", "--single-branch", "--depth", "1", repoURL, repoDir).CombinedOutput(); err != nil {
		t.Fatalf("git clone failed: %v: %s", err, string(out))
	}

	// Create input tar
	inputTar := filepath.Join(work, "input.tar")
	if out, err := exec.Command("tar", "-C", repoDir, "-cf", inputTar, ".").CombinedOutput(); err != nil {
		t.Fatalf("tar create failed: %v: %s", err, string(out))
	}

	// Upload input tar to SeaweedFS
	ts := time.Now().Unix()
	inputKey := fmt.Sprintf("transflow/docker-smoke/%d/input.tar", ts)
	inputURL := fmt.Sprintf("%s/artifacts/%s", trimRight(seaweed, "/"), inputKey)
	f, err := os.Open(inputTar)
	if err != nil {
		t.Fatalf("open input.tar: %v", err)
	}
	defer f.Close()
	req, _ := http.NewRequest(http.MethodPut, inputURL, f)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload input.tar failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("upload input.tar http %d", resp.StatusCode)
	}

	// Run container
	diffKey := fmt.Sprintf("transflow/docker-smoke/%d/diff.patch", ts)
	cmd := exec.Command("docker", "run", "--rm", "--network", "host",
		"-e", "INPUT_URL="+inputURL,
		"-e", "RECIPE="+recipeClass,
		"-e", "RECIPE_GROUP="+recipeGroup,
		"-e", "RECIPE_ARTIFACT="+recipeArtifact,
		"-e", "RECIPE_VERSION="+recipeVersion,
		"-e", "MAVEN_PLUGIN_VERSION="+pluginVersion,
		"-e", "SEAWEEDFS_URL="+seaweed,
		"-e", "DIFF_KEY="+diffKey,
		image,
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run() // container may exit non-zero; we rely on diff existence for pass/fail

	// Fetch diff
	diffURL := fmt.Sprintf("%s/artifacts/%s", trimRight(seaweed, "/"), diffKey)
	r, err := http.Get(diffURL)
	if err != nil {
		t.Fatalf("fetch diff: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatalf("diff not found: http %d; logs:\n%s", r.StatusCode, buf.String())
	}
	n, _ := io.Copy(io.Discard, r.Body)
	if n == 0 {
		t.Fatalf("empty diff; logs:\n%s", buf.String())
	}
}

func getenvDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func trimRight(s, cut string) string {
	for len(s) > 0 && len(cut) > 0 && s[len(s)-1] == cut[0] {
		s = s[:len(s)-1]
	}
	return s
}

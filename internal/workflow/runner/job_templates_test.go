package runner

import (
	"os"
	"testing"
)

func TestDockerHubNamespaceResolution(t *testing.T) {
	t.Setenv("DOCKERHUB_USERNAME", "exampleuser")
	if got := dockerHubNamespace(); got != "docker.io/exampleuser" {
		t.Fatalf("expected docker.io/exampleuser, got %s", got)
	}

	// DOCKERHUB_USERNAME wins over MODS_IMAGE_PREFIX
	t.Setenv("MODS_IMAGE_PREFIX", "docker.io/otherorg")
	if got := dockerHubNamespace(); got != "docker.io/exampleuser" {
		t.Fatalf("DOCKERHUB_USERNAME should take precedence, got %s", got)
	}

	// When DOCKERHUB_USERNAME is unset, use MODS_IMAGE_PREFIX if provided
	os.Unsetenv("DOCKERHUB_USERNAME")
	if got := dockerHubNamespace(); got != "docker.io/otherorg" {
		t.Fatalf("expected docker.io/otherorg from MODS_IMAGE_PREFIX, got %s", got)
	}
}

func TestRegistryImageBuildsDockerHubRef(t *testing.T) {
	t.Setenv("DOCKERHUB_USERNAME", "acme")
	img := registryImage("mods-plan")
	if img != "docker.io/acme/mods-plan:latest" {
		t.Fatalf("unexpected image: %s", img)
	}
}

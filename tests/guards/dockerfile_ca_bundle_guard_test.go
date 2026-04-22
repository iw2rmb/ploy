package guards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishedImageDockerfilesUseBuildCABundleInstaller(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	dockerfiles := []string{
		"images/server/Dockerfile",
		"images/node/Dockerfile",
		"images/amata/Dockerfile",
		"images/java-17-orw-codex-amata/Dockerfile",
		"images/sbom/gradle/Dockerfile",
		"images/sbom/maven/Dockerfile",
		"images/gates/gradle/Dockerfile.jdk11",
		"images/gates/gradle/Dockerfile.jdk17",
		"images/gates/maven/Dockerfile.jdk11",
		"images/gates/maven/Dockerfile.jdk17",
		"images/orw/orw-cli-maven/Dockerfile",
		"images/orw/orw-cli-gradle/Dockerfile",
	}

	for _, rel := range dockerfiles {
		path := filepath.Join(repoRoot, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(data)
		if !strings.Contains(text, "id=ploy_ca_certs") {
			t.Fatalf("%s: missing BuildKit CA secret mount id=ploy_ca_certs", rel)
		}
		if !strings.Contains(text, "images/install_ploy_ca_bundle.sh") {
			t.Fatalf("%s: missing shared installer copy from images/install_ploy_ca_bundle.sh", rel)
		}
		if !strings.Contains(text, "install_ploy_ca_bundle") {
			t.Fatalf("%s: missing install_ploy_ca_bundle invocation", rel)
		}
	}
}

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
		"images/java-bases/gradle/Dockerfile.jdk11",
		"images/java-bases/gradle/Dockerfile.jdk17",
		// jdk21/jdk25 Java-base images use internal Artifactory base images
		// that do not require additional build-time CA bootstrap.
		"images/java-bases/maven/Dockerfile.jdk11",
		"images/java-bases/maven/Dockerfile.jdk17",
		"images/java-bases/temurin/Dockerfile.jdk17",
		"images/amata/amata-codex-java-17-maven/Dockerfile",
		"images/amata/amata-codex-java-17-gradle/Dockerfile",
		// jdk21/jdk25 Amata images do not use build-time CA bootstrap.
		"images/orw/orw-cli-java-17-maven/Dockerfile",
		"images/orw/orw-cli-java-17-gradle/Dockerfile",
		// jdk21/jdk25 ORW images do not use build-time CA bootstrap.
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

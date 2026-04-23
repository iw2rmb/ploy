package guards

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type runtimeEnvPolicy struct {
	RequireLocale      bool
	RequireMavenConfig bool
	RequireGradleHome  bool
}

func TestDockerfilesRuntimeEnvPolicy(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	policy := map[string]runtimeEnvPolicy{
		"images/java-bases/maven/Dockerfile.jdk11":     {RequireLocale: true, RequireMavenConfig: true},
		"images/java-bases/maven/Dockerfile.jdk17":     {RequireLocale: true, RequireMavenConfig: true},
		"images/java-bases/gradle/Dockerfile.jdk11":    {RequireLocale: true, RequireGradleHome: true},
		"images/java-bases/gradle/Dockerfile.jdk17":    {RequireLocale: true, RequireGradleHome: true},
		"images/java-bases/temurin/Dockerfile.jdk17":   {RequireLocale: true},
		"images/gates/gradle/Dockerfile.jdk11":         {RequireGradleHome: true},
		"images/gates/gradle/Dockerfile.jdk17":         {RequireGradleHome: true},
		"images/gates/maven/Dockerfile.jdk11":          {RequireMavenConfig: true},
		"images/gates/maven/Dockerfile.jdk17":          {RequireMavenConfig: true},
		"images/sbom/gradle/Dockerfile.jdk11":          {RequireGradleHome: true},
		"images/sbom/gradle/Dockerfile.jdk17":          {RequireGradleHome: true},
		"images/sbom/maven/Dockerfile.jdk11":           {RequireMavenConfig: true},
		"images/sbom/maven/Dockerfile.jdk17":           {RequireMavenConfig: true},
		"images/orw/orw-cli-gradle/Dockerfile":         {},
		"images/orw/orw-cli-maven/Dockerfile":          {},
		"images/java-17-codex-amata-maven/Dockerfile":  {RequireLocale: true, RequireMavenConfig: true},
		"images/java-17-codex-amata-gradle/Dockerfile": {RequireLocale: true, RequireGradleHome: true},
	}

	for rel, spec := range policy {
		path := filepath.Join(repoRoot, rel)
		envs := parseDockerfileEnvAssignments(t, path)

		if spec.RequireLocale {
			if got := envs["LANG"]; got != "C.UTF-8" {
				t.Fatalf("%s: LANG=%q, want %q", rel, got, "C.UTF-8")
			}
			if got := envs["LC_ALL"]; got != "C.UTF-8" {
				t.Fatalf("%s: LC_ALL=%q, want %q", rel, got, "C.UTF-8")
			}
		}
		if spec.RequireMavenConfig {
			if got := envs["MAVEN_CONFIG"]; got != "/root/.m2" {
				t.Fatalf("%s: MAVEN_CONFIG=%q, want %q", rel, got, "/root/.m2")
			}
		}
		if spec.RequireGradleHome {
			if got := envs["GRADLE_USER_HOME"]; got != "/root/.gradle" {
				t.Fatalf("%s: GRADLE_USER_HOME=%q, want %q", rel, got, "/root/.gradle")
			}
		}
	}
}

func parseDockerfileEnvAssignments(t *testing.T, path string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(data), "\n")
	pairPattern := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)=("[^"]*"|[^"\s\\]+)`)

	envs := make(map[string]string, 8)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(strings.ToUpper(line), "ENV ") {
			continue
		}

		expr := strings.TrimSpace(line[4:])
		for strings.HasSuffix(strings.TrimSpace(lines[i]), "\\") && i+1 < len(lines) {
			expr = strings.TrimSuffix(strings.TrimSpace(expr), "\\")
			i++
			expr += " " + strings.TrimSpace(lines[i])
		}

		matches := pairPattern.FindAllStringSubmatch(expr, -1)
		for _, match := range matches {
			value := strings.Trim(match[2], "\"")
			envs[match[1]] = value
		}
	}

	return envs
}

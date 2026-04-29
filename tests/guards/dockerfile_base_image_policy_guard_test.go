package guards

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type runtimeBasePolicy struct {
	Expected string
	Reason   string
}

func TestDockerfilesRuntimeBasePolicy(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	dockerfiles := findRepositoryDockerfiles(t, filepath.Join(repoRoot, "images"))
	if len(dockerfiles) == 0 {
		t.Fatal("no Dockerfiles found under images/")
	}

	// Debian-focused policy with slim where possible. Non-slim runtime bases are
	// explicitly allowlisted when upstream toolchain images do not provide slim.
	policy := map[string]runtimeBasePolicy{
		"images/server/Dockerfile":                     {Expected: "debian:bookworm-slim", Reason: "core runtime must be Debian slim"},
		"images/node/Dockerfile":                       {Expected: "debian:bookworm-slim", Reason: "core runtime must be Debian slim"},
		"images/amata/amata-codex-java-17-maven/Dockerfile":  {Expected: "node:22-bookworm-slim", Reason: "Node runtime provides official Debian slim tag"},
		"images/amata/amata-codex-java-17-gradle/Dockerfile": {Expected: "node:22-bookworm-slim", Reason: "Node runtime provides official Debian slim tag"},
		"images/amata/amata-codex-java-21-maven/Dockerfile":  {Expected: "node:22-bookworm-slim", Reason: "Node runtime provides official Debian slim tag"},
		"images/amata/amata-codex-java-21-gradle/Dockerfile": {Expected: "node:22-bookworm-slim", Reason: "Node runtime provides official Debian slim tag"},
		"images/amata/amata-codex-java-25-maven/Dockerfile":  {Expected: "node:22-bookworm-slim", Reason: "Node runtime provides official Debian slim tag"},
		"images/amata/amata-codex-java-25-gradle/Dockerfile": {Expected: "node:22-bookworm-slim", Reason: "Node runtime provides official Debian slim tag"},
		"images/java-bases/maven/Dockerfile.jdk11":     {Expected: "maven:3.9.11-eclipse-temurin-11", Reason: "exception: no official Maven Eclipse Temurin slim tag"},
		"images/java-bases/maven/Dockerfile.jdk17":     {Expected: "maven:3.9.11-eclipse-temurin-17", Reason: "exception: no official Maven Eclipse Temurin slim tag"},
		"images/java-bases/maven/Dockerfile.jdk21":     {Expected: "docker-hosted.artifactory.tcsbank.ru/sec-base-images/ubuntu-noble-java-adoptium-21-cicd:latest", Reason: "internal cicd base with approved trust and repo routing"},
		"images/java-bases/maven/Dockerfile.jdk25":     {Expected: "docker-hosted.artifactory.tcsbank.ru/sec-base-images/ubuntu-noble-java-adoptium-25-cicd:latest", Reason: "internal cicd base with approved trust and repo routing"},
		"images/java-bases/gradle/Dockerfile.jdk11":    {Expected: "gradle:8.8-jdk11", Reason: "exception: no official gradle:8.8-jdk11-slim tag"},
		"images/java-bases/gradle/Dockerfile.jdk17":    {Expected: "gradle:8.8-jdk17", Reason: "exception: no official gradle:8.8-jdk17-slim tag"},
		"images/java-bases/gradle/Dockerfile.jdk21":    {Expected: "docker-hosted.artifactory.tcsbank.ru/sec-base-images/ubuntu-noble-java-adoptium-21-cicd:latest", Reason: "internal cicd base with approved trust and repo routing"},
		"images/java-bases/gradle/Dockerfile.jdk25":    {Expected: "docker-hosted.artifactory.tcsbank.ru/sec-base-images/ubuntu-noble-java-adoptium-25-cicd:latest", Reason: "internal cicd base with approved trust and repo routing"},
		"images/java-bases/temurin/Dockerfile.jdk17":   {Expected: "eclipse-temurin:17-jdk", Reason: "exception: no official eclipse-temurin:17-jdk-slim tag"},
		"images/java-bases/temurin/Dockerfile.jdk21":   {Expected: "docker-hosted.artifactory.tcsbank.ru/sec-base-images/ubuntu-noble-java-adoptium-21-cicd:latest", Reason: "internal cicd base with approved trust and repo routing"},
		"images/java-bases/temurin/Dockerfile.jdk25":   {Expected: "docker-hosted.artifactory.tcsbank.ru/sec-base-images/ubuntu-noble-java-adoptium-25-cicd:latest", Reason: "internal cicd base with approved trust and repo routing"},
		"images/gates/gradle/Dockerfile.jdk11":         {Expected: "java-base-gradle:jdk11", Reason: "inherits unified CA/toolchain lane"},
		"images/gates/gradle/Dockerfile.jdk17":         {Expected: "java-base-gradle:jdk17", Reason: "inherits unified CA/toolchain lane"},
		"images/gates/gradle/Dockerfile.jdk21":         {Expected: "java-base-gradle:jdk21", Reason: "inherits unified CA/toolchain lane"},
		"images/gates/gradle/Dockerfile.jdk25":         {Expected: "java-base-gradle:jdk25", Reason: "inherits unified CA/toolchain lane"},
		"images/gates/maven/Dockerfile.jdk11":          {Expected: "java-base-maven:jdk11", Reason: "inherits unified CA/toolchain lane"},
		"images/gates/maven/Dockerfile.jdk17":          {Expected: "java-base-maven:jdk17", Reason: "inherits unified CA/toolchain lane"},
		"images/gates/maven/Dockerfile.jdk21":          {Expected: "java-base-maven:jdk21", Reason: "inherits unified CA/toolchain lane"},
		"images/gates/maven/Dockerfile.jdk25":          {Expected: "java-base-maven:jdk25", Reason: "inherits unified CA/toolchain lane"},
		"images/orw/orw-cli-java-17-gradle/Dockerfile":         {Expected: "java-base-temurin:jdk17", Reason: "inherits unified CA/toolchain lane"},
		"images/orw/orw-cli-java-21-gradle/Dockerfile":   {Expected: "java-base-temurin:jdk21", Reason: "inherits unified CA/toolchain lane"},
		"images/orw/orw-cli-java-25-gradle/Dockerfile":   {Expected: "java-base-temurin:jdk25", Reason: "inherits unified CA/toolchain lane"},
		"images/orw/orw-cli-java-17-maven/Dockerfile":          {Expected: "java-base-temurin:jdk17", Reason: "inherits unified CA/toolchain lane"},
		"images/orw/orw-cli-java-21-maven/Dockerfile":    {Expected: "java-base-temurin:jdk21", Reason: "inherits unified CA/toolchain lane"},
		"images/orw/orw-cli-java-25-maven/Dockerfile":    {Expected: "java-base-temurin:jdk25", Reason: "inherits unified CA/toolchain lane"},
	}

	found := make([]string, 0, len(dockerfiles))
	for _, path := range dockerfiles {
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			t.Fatalf("rel path %s: %v", path, err)
		}
		rel = filepath.ToSlash(rel)
		found = append(found, rel)
	}
	slices.Sort(found)

	expected := make([]string, 0, len(policy))
	for rel := range policy {
		expected = append(expected, rel)
	}
	slices.Sort(expected)

	if !slices.Equal(found, expected) {
		t.Fatalf("policy coverage mismatch:\nfound:    %v\nexpected: %v", found, expected)
	}

	for _, path := range dockerfiles {
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			t.Fatalf("rel path %s: %v", path, err)
		}
		rel = filepath.ToSlash(rel)
		spec := policy[rel]
		finalRef := parseDockerfileFinalFromRef(t, path)
		resolved := resolveFromArgs(parseDockerfileArgDefaults(t, path), finalRef)

		if strings.Contains(strings.ToLower(resolved), "alpine") {
			t.Fatalf("%s: final runtime base must not be alpine (got %q)", rel, resolved)
		}
		if resolved != spec.Expected {
			t.Fatalf("%s: final runtime base=%q, want %q (%s)", rel, resolved, spec.Expected, spec.Reason)
		}
	}
}

func parseDockerfileFinalFromRef(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(data), "\n")
	fromRe := regexp.MustCompile(`(?i)^FROM\s+(?:--platform=\S+\s+)?(\S+)`)

	finalRef := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		match := fromRe.FindStringSubmatch(trimmed)
		if len(match) == 2 {
			finalRef = match[1]
		}
	}

	if finalRef == "" {
		t.Fatalf("%s: no FROM instruction found", path)
	}
	return finalRef
}

func parseDockerfileArgDefaults(t *testing.T, path string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(data), "\n")
	argRe := regexp.MustCompile(`(?i)^ARG\s+([A-Za-z_][A-Za-z0-9_]*)=(\S+)\s*$`)

	args := make(map[string]string)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		match := argRe.FindStringSubmatch(trimmed)
		if len(match) != 3 {
			continue
		}
		args[match[1]] = match[2]
	}
	return args
}

func resolveFromArgs(argDefaults map[string]string, fromRef string) string {
	resolved := fromRef
	for key, val := range argDefaults {
		resolved = strings.ReplaceAll(resolved, "${"+key+"}", val)
		resolved = strings.ReplaceAll(resolved, "$"+key, val)
	}
	return resolved
}

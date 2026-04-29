package guards

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	requiredOCISourceValue   = "https://github.com/iw2rmb/ploy"
	requiredOCILicensesValue = "MIT"
)

func TestDockerfilesOCIRequiredLabels(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	dockerfiles := findRepositoryDockerfiles(t, filepath.Join(repoRoot, "images"))
	if len(dockerfiles) == 0 {
		t.Fatal("no Dockerfiles found under images/")
	}

	requiredKeys := []string{
		"org.opencontainers.image.description",
	}

	noSourceLicense := map[string]struct{}{
		"images/java-bases/maven/Dockerfile.jdk21":         {},
		"images/java-bases/maven/Dockerfile.jdk25":         {},
		"images/java-bases/gradle/Dockerfile.jdk21":        {},
		"images/java-bases/gradle/Dockerfile.jdk25":        {},
		"images/java-bases/temurin/Dockerfile.jdk21":       {},
		"images/java-bases/temurin/Dockerfile.jdk25":       {},
		"images/gates/maven/Dockerfile.jdk21":              {},
		"images/gates/maven/Dockerfile.jdk25":              {},
		"images/gates/gradle/Dockerfile.jdk21":             {},
		"images/gates/gradle/Dockerfile.jdk25":             {},
		"images/orw/orw-cli-java-21-maven/Dockerfile":      {},
		"images/orw/orw-cli-java-25-maven/Dockerfile":      {},
		"images/orw/orw-cli-java-21-gradle/Dockerfile":     {},
		"images/orw/orw-cli-java-25-gradle/Dockerfile":     {},
		"images/amata/amata-codex-java-21-maven/Dockerfile":  {},
		"images/amata/amata-codex-java-25-maven/Dockerfile":  {},
		"images/amata/amata-codex-java-21-gradle/Dockerfile": {},
		"images/amata/amata-codex-java-25-gradle/Dockerfile": {},
	}

	for _, dockerfile := range dockerfiles {
		rel, err := filepath.Rel(repoRoot, dockerfile)
		if err != nil {
			t.Fatalf("rel path %s: %v", dockerfile, err)
		}
		rel = filepath.ToSlash(rel)

		labels := parseDockerfileLabels(t, dockerfile)
		keyCounts := make(map[string]int, len(labels))
		for _, label := range labels {
			if strings.HasPrefix(label.Key, "org.opencontainers.image.") {
				keyCounts[label.Key]++
			}
		}

		for key, count := range keyCounts {
			if count > 1 {
				t.Fatalf("%s: duplicate OCI label key %q (%d occurrences)", dockerfile, key, count)
			}
		}
		for _, key := range requiredKeys {
			if keyCounts[key] != 1 {
				t.Fatalf("%s: required OCI label %q count=%d, want 1", rel, key, keyCounts[key])
			}
		}

		_, allowMissingSourceLicense := noSourceLicense[rel]
		sourceCount := keyCounts["org.opencontainers.image.source"]
		licensesCount := keyCounts["org.opencontainers.image.licenses"]
		if allowMissingSourceLicense {
			if sourceCount > 0 {
				t.Fatalf("%s: org.opencontainers.image.source must be absent", rel)
			}
			if licensesCount > 0 {
				t.Fatalf("%s: org.opencontainers.image.licenses must be absent", rel)
			}
		} else {
			if sourceCount != 1 {
				t.Fatalf("%s: required OCI label %q count=%d, want 1", rel, "org.opencontainers.image.source", sourceCount)
			}
			if licensesCount != 1 {
				t.Fatalf("%s: required OCI label %q count=%d, want 1", rel, "org.opencontainers.image.licenses", licensesCount)
			}

			source := labelValue(labels, "org.opencontainers.image.source")
			if source != requiredOCISourceValue {
				t.Fatalf("%s: org.opencontainers.image.source=%q, want %q", rel, source, requiredOCISourceValue)
			}

			licenses := labelValue(labels, "org.opencontainers.image.licenses")
			if licenses != requiredOCILicensesValue {
				t.Fatalf("%s: org.opencontainers.image.licenses=%q, want %q", rel, licenses, requiredOCILicensesValue)
			}
		}

		description := strings.TrimSpace(labelValue(labels, "org.opencontainers.image.description"))
		if description == "" {
			t.Fatalf("%s: org.opencontainers.image.description must be non-empty", rel)
		}
		if strings.Contains(description, "\n") || strings.Contains(description, "\r") {
			t.Fatalf("%s: org.opencontainers.image.description must be single-line", rel)
		}
	}
}

type dockerLabel struct {
	Key   string
	Value string
}

func findRepositoryDockerfiles(t *testing.T, root string) []string {
	t.Helper()

	var dockerfiles []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), "Dockerfile") {
			dockerfiles = append(dockerfiles, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	slices.Sort(dockerfiles)
	return dockerfiles
}

func mustFindRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		modPath := filepath.Join(dir, "go.mod")
		imagesPath := filepath.Join(dir, "images")
		if _, err := os.Stat(modPath); err == nil {
			if st, err := os.Stat(imagesPath); err == nil && st.IsDir() {
				return dir
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repository root not found from %q", dir)
		}
		dir = parent
	}
}

func parseDockerfileLabels(t *testing.T, path string) []dockerLabel {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(data), "\n")

	pairPattern := regexp.MustCompile(`([A-Za-z0-9._-]+)\s*=\s*("(?:[^"\\]|\\.)*"|[^"\s]+)`)
	labels := make([]dockerLabel, 0, 8)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(strings.ToUpper(line), "LABEL ") {
			continue
		}

		expr := strings.TrimSpace(line[6:])
		for strings.HasSuffix(strings.TrimSpace(lines[i]), "\\") && i+1 < len(lines) {
			expr = strings.TrimSuffix(strings.TrimSpace(expr), "\\")
			i++
			expr += " " + strings.TrimSpace(lines[i])
		}

		matches := pairPattern.FindAllStringSubmatch(expr, -1)
		for _, m := range matches {
			value := m[2]
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				if unquoted, err := strconv.Unquote(value); err == nil {
					value = unquoted
				}
			}
			labels = append(labels, dockerLabel{
				Key:   m[1],
				Value: value,
			})
		}
	}

	return labels
}

func labelValue(labels []dockerLabel, key string) string {
	for _, label := range labels {
		if label.Key == key {
			return label.Value
		}
	}
	return ""
}

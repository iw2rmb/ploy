package python

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DetectVersion tries to determine Python version from runtime.txt, .python-version, pyproject.toml, or Pipfile
func DetectVersion(srcDir string) string {
	if v := strings.TrimSpace(read(filepath.Join(srcDir, "runtime.txt"))); v != "" {
		return normalize(v)
	}
	if v := strings.TrimSpace(read(filepath.Join(srcDir, ".python-version"))); v != "" {
		return normalize(v)
	}
	if v := detectFromPyProject(filepath.Join(srcDir, "pyproject.toml")); v != "" {
		return v
	}
	if v := detectFromPipfile(filepath.Join(srcDir, "Pipfile")); v != "" {
		return v
	}
	return ""
}

func read(path string) string { b, _ := os.ReadFile(path); return string(b) }

func normalize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "python-")
	return s
}

func detectFromPyProject(path string) string {
	txt := read(path)
	if txt == "" {
		return ""
	}
	// [project] requires-python = ">=3.10"
	if re := regexp.MustCompile(`requires-python\s*=\s*"([^"]+)"`); re.MatchString(txt) {
		v := re.FindStringSubmatch(txt)[1]
		return strings.TrimLeft(v, ">=~^")
	}
	// [tool.poetry.dependencies] python = ">=3.10,<4.0"
	if re := regexp.MustCompile(`python\s*=\s*"([^"]+)"`); re.MatchString(txt) {
		v := re.FindStringSubmatch(txt)[1]
		return strings.TrimLeft(strings.Split(v, ",")[0], ">=~^")
	}
	return ""
}

func detectFromPipfile(path string) string {
	txt := read(path)
	if txt == "" {
		return ""
	}
	if re := regexp.MustCompile(`python_version\s*=\s*"([^"]+)"`); re.MatchString(txt) {
		return re.FindStringSubmatch(txt)[1]
	}
	return ""
}

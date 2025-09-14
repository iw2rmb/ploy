package node

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DetectVersion tries to determine Node.js version from package.json engines, .nvmrc, or .node-version
func DetectVersion(srcDir string) string {
	if v := detectFromPackageJSON(filepath.Join(srcDir, "package.json")); v != "" {
		return v
	}
	if v := strings.TrimSpace(read(filepath.Join(srcDir, ".nvmrc"))); v != "" {
		return normalize(v)
	}
	if v := strings.TrimSpace(read(filepath.Join(srcDir, ".node-version"))); v != "" {
		return normalize(v)
	}
	return ""
}

func read(path string) string { b, _ := os.ReadFile(path); return string(b) }

func detectFromPackageJSON(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pkg struct {
		Engines map[string]string `json:"engines"`
	}
	if err := json.Unmarshal(b, &pkg); err == nil {
		if e, ok := pkg.Engines["node"]; ok {
			return normalize(e)
		}
	}
	return ""
}

func normalize(s string) string {
	s = strings.TrimSpace(s)
	// Remove operators like ">=", "^", "~", "v"
	s = strings.TrimLeft(s, ">=^~v")
	// Extract major.minor if present
	re := regexp.MustCompile(`^(\d+)(\.\d+)?`)
	if re.MatchString(s) {
		return strings.TrimPrefix(re.FindString(s), "v")
	}
	return s
}

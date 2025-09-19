package git

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetRepositoryURL returns the repository URL from various sources.
func (g *GitUtils) GetRepositoryURL() (string, error) {
	if url := g.tryGetRemoteURL(); url != "" {
		return url, nil
	}
	if url := g.tryParseGitConfig(); url != "" {
		return url, nil
	}
	if url := g.tryExtractFromPackageJSON(); url != "" {
		return url, nil
	}
	if url := g.tryExtractFromCargoToml(); url != "" {
		return url, nil
	}
	if url := g.tryExtractFromPomXML(); url != "" {
		return url, nil
	}
	if url := g.tryExtractFromGoMod(); url != "" {
		return url, nil
	}

	return "", fmt.Errorf("unable to determine repository URL")
}

func (g *GitUtils) tryGetRemoteURL() string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = g.workingDir

	if output, err := cmd.Output(); err == nil {
		url := strings.TrimSpace(string(output))
		return g.normalizeURL(url)
	}
	return ""
}

func (g *GitUtils) tryParseGitConfig() string {
	configPath := filepath.Join(g.workingDir, ".git", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "[remote \"origin\"]" {
			for j := i + 1; j < len(lines); j++ {
				entry := strings.TrimSpace(lines[j])
				if entry == "" || strings.HasPrefix(entry, "[") {
					break
				}
				if strings.HasPrefix(entry, "url =") {
					parts := strings.SplitN(entry, "=", 2)
					if len(parts) == 2 {
						return g.normalizeURL(strings.TrimSpace(parts[1]))
					}
				}
			}
		}
	}
	return ""
}

func (g *GitUtils) tryExtractFromPackageJSON() string {
	pkgPath := filepath.Join(g.workingDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return ""
	}

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}

	if repo, ok := pkg["repository"]; ok {
		if repoMap, ok := repo.(map[string]interface{}); ok {
			if url, ok := repoMap["url"].(string); ok {
				return g.normalizeURL(url)
			}
		} else if repoStr, ok := repo.(string); ok {
			return g.normalizeURL(repoStr)
		}
	}
	return ""
}

func (g *GitUtils) tryExtractFromCargoToml() string {
	cargoPath := filepath.Join(g.workingDir, "Cargo.toml")
	data, err := os.ReadFile(cargoPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "repository") && strings.Contains(line, "=") {
			parts := strings.Split(line, "=")
			if len(parts) > 1 {
				url := strings.Trim(strings.TrimSpace(parts[1]), `"`)
				return g.normalizeURL(url)
			}
		}
	}
	return ""
}

func (g *GitUtils) tryExtractFromPomXML() string {
	pomPath := filepath.Join(g.workingDir, "pom.xml")
	data, err := os.ReadFile(pomPath)
	if err != nil {
		return ""
	}

	content := string(data)
	patterns := []string{
		`<scm>.*?<url>(.*?)</url>.*?</scm>`,
		`<scm>.*?<connection>(.*?)</connection>.*?</scm>`,
	}

	for _, pattern := range patterns {
		if matches := findStringSubmatch(pattern, content); len(matches) > 1 {
			url := strings.TrimSpace(matches[1])
			url = strings.TrimPrefix(url, "scm:git:")
			return g.normalizeURL(url)
		}
	}
	return ""
}

func (g *GitUtils) tryExtractFromGoMod() string {
	goModPath := filepath.Join(g.workingDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				modulePath := parts[1]
				if strings.Contains(modulePath, "github.com") ||
					strings.Contains(modulePath, "gitlab.com") ||
					strings.Contains(modulePath, "bitbucket.org") {
					return g.normalizeURL("https://" + modulePath)
				}
			}
		}
	}
	return ""
}

func (g *GitUtils) normalizeURL(url string) string {
	url = strings.TrimSpace(url)
	if strings.HasPrefix(url, "git@") {
		url = strings.ReplaceAll(url, "git@", "https://")
		url = strings.ReplaceAll(url, ":", "/")
	}
	url = strings.TrimSuffix(url, ".git")
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		if strings.Contains(url, "github.com") || strings.Contains(url, "gitlab.com") || strings.Contains(url, "bitbucket.org") {
			url = "https://" + url
		}
	}
	return url
}

func findStringSubmatch(pattern, text string) []string {
	if pattern == "" {
		return []string{text, ""}
	}
	if strings.Contains(text, pattern) {
		return []string{text, pattern}
	}
	return nil
}

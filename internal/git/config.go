package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// getRepositoryURL extracts the repository URL from multiple sources
func (r *Repository) getRepositoryURL() (string, error) {
	// Try git remote get-url origin first
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = r.Path

	if output, err := cmd.Output(); err == nil {
		url := strings.TrimSpace(string(output))
		if url != "" {
			return r.normalizeRepositoryURL(url), nil
		}
	}

	// Fall back to parsing .git/config
	return r.parseGitConfig()
}

// parseGitConfig parses .git/config file for repository URL
func (r *Repository) parseGitConfig() (string, error) {
	configPath := filepath.Join(r.Path, ".git", "config")
	file, err := os.Open(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git config: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inOriginSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.Contains(line, `[remote "origin"]`) {
			inOriginSection = true
			continue
		}

		if strings.HasPrefix(line, "[") && !strings.Contains(line, `[remote "origin"]`) {
			inOriginSection = false
			continue
		}

		if inOriginSection && strings.Contains(line, "url =") {
			parts := strings.Split(line, "url =")
			if len(parts) > 1 {
				url := strings.TrimSpace(parts[1])
				return r.normalizeRepositoryURL(url), nil
			}
		}
	}

	return "", fmt.Errorf("no origin URL found in git config")
}

// normalizeRepositoryURL normalizes repository URLs to a standard format
func (r *Repository) normalizeRepositoryURL(url string) string {
	// Convert SSH URLs to HTTPS format for consistency
	sshPattern := regexp.MustCompile(`git@([^:]+):(.+)\.git$`)
	if matches := sshPattern.FindStringSubmatch(url); len(matches) == 3 {
		return fmt.Sprintf("https://%s/%s", matches[1], matches[2])
	}

	// Remove .git suffix if present
	url = strings.TrimSuffix(url, ".git")

	return url
}

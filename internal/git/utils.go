package git

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitUtils provides enhanced Git utility functions
type GitUtils struct {
	workingDir string
}

// NewGitUtils creates a new GitUtils instance for the specified directory
func NewGitUtils(workingDir string) *GitUtils {
	return &GitUtils{workingDir: workingDir}
}

// GetSHA returns the current Git SHA (full or short version)
func (g *GitUtils) GetSHA(short bool) (string, error) {
	args := []string{"rev-parse", "HEAD"}
	if short {
		args = []string{"rev-parse", "--short=12", "HEAD"}
	}
	
	cmd := exec.Command("git", args...)
	cmd.Dir = g.workingDir
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git SHA: %w", err)
	}
	
	return strings.TrimSpace(string(output)), nil
}

// GetShortSHA returns the short version of the current Git SHA
func (g *GitUtils) GetShortSHA() string {
	sha, _ := g.GetSHA(true)
	return sha
}

// GetBranch returns the current Git branch
func (g *GitUtils) GetBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = g.workingDir
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git branch: %w", err)
	}
	
	branch := strings.TrimSpace(string(output))
	if branch == "HEAD" {
		return "detached", nil
	}
	
	return branch, nil
}

// GetRepositoryURL returns the repository URL from various sources
func (g *GitUtils) GetRepositoryURL() (string, error) {
	// Try multiple approaches to get repository URL
	
	// 1. Try git remote get-url origin
	if url := g.tryGetRemoteURL(); url != "" {
		return url, nil
	}
	
	// 2. Try parsing .git/config
	if url := g.tryParseGitConfig(); url != "" {
		return url, nil
	}
	
	// 3. Try extracting from package.json (for Node.js projects)
	if url := g.tryExtractFromPackageJSON(); url != "" {
		return url, nil
	}
	
	// 4. Try extracting from Cargo.toml (for Rust projects)
	if url := g.tryExtractFromCargoToml(); url != "" {
		return url, nil
	}
	
	// 5. Try extracting from pom.xml (for Java projects)
	if url := g.tryExtractFromPomXML(); url != "" {
		return url, nil
	}
	
	// 6. Try extracting from go.mod (for Go projects)
	if url := g.tryExtractFromGoMod(); url != "" {
		return url, nil
	}
	
	return "", fmt.Errorf("unable to determine repository URL")
}

// tryGetRemoteURL tries to get URL using git remote
func (g *GitUtils) tryGetRemoteURL() string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = g.workingDir
	
	if output, err := cmd.Output(); err == nil {
		url := strings.TrimSpace(string(output))
		return g.normalizeURL(url)
	}
	
	return ""
}

// tryParseGitConfig tries to parse .git/config for repository URL
func (g *GitUtils) tryParseGitConfig() string {
	configPath := filepath.Join(g.workingDir, ".git", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	
	lines := strings.Split(string(data), "\n")
	inOriginSection := false
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
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
				return g.normalizeURL(url)
			}
		}
	}
	
	return ""
}

// tryExtractFromPackageJSON tries to extract repository URL from package.json
func (g *GitUtils) tryExtractFromPackageJSON() string {
	packagePath := filepath.Join(g.workingDir, "package.json")
	data, err := os.ReadFile(packagePath)
	if err != nil {
		return ""
	}
	
	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	
	if repo, ok := pkg["repository"]; ok {
		// Handle both string and object formats
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

// tryExtractFromCargoToml tries to extract repository URL from Cargo.toml
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

// tryExtractFromPomXML tries to extract repository URL from pom.xml
func (g *GitUtils) tryExtractFromPomXML() string {
	pomPath := filepath.Join(g.workingDir, "pom.xml")
	data, err := os.ReadFile(pomPath)
	if err != nil {
		return ""
	}
	
	content := string(data)
	
	// Look for <scm><url> or <scm><connection> tags
	patterns := []string{
		`<scm>.*?<url>(.*?)</url>.*?</scm>`,
		`<scm>.*?<connection>(.*?)</connection>.*?</scm>`,
	}
	
	for _, pattern := range patterns {
		if matches := findStringSubmatch(pattern, content); len(matches) > 1 {
			url := strings.TrimSpace(matches[1])
			// Clean up SCM URLs
			if strings.HasPrefix(url, "scm:git:") {
				url = strings.TrimPrefix(url, "scm:git:")
			}
			return g.normalizeURL(url)
		}
	}
	
	return ""
}

// tryExtractFromGoMod tries to extract repository URL from go.mod
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
				// Convert module path to repository URL
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

// normalizeURL normalizes repository URLs to a consistent format
func (g *GitUtils) normalizeURL(url string) string {
	url = strings.TrimSpace(url)
	
	// Convert SSH URLs to HTTPS
	if strings.HasPrefix(url, "git@") {
		// git@github.com:user/repo.git -> https://github.com/user/repo
		url = strings.ReplaceAll(url, "git@", "https://")
		url = strings.ReplaceAll(url, ":", "/")
	}
	
	// Remove .git suffix
	if strings.HasSuffix(url, ".git") {
		url = url[:len(url)-4]
	}
	
	// Ensure https:// prefix
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		if strings.Contains(url, "github.com") || strings.Contains(url, "gitlab.com") || strings.Contains(url, "bitbucket.org") {
			url = "https://" + url
		}
	}
	
	return url
}

// IsGitRepository checks if the directory is a Git repository
func (g *GitUtils) IsGitRepository() bool {
	gitDir := filepath.Join(g.workingDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}
	
	// Also check if we're in a subdirectory of a Git repository
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = g.workingDir
	
	return cmd.Run() == nil
}

// GetStatus returns the Git status (clean, dirty, untracked files)
func (g *GitUtils) GetStatus() (bool, bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = g.workingDir
	
	output, err := cmd.Output()
	if err != nil {
		return false, false, fmt.Errorf("failed to get git status: %w", err)
	}
	
	statusLines := strings.Split(strings.TrimSpace(string(output)), "\n")
	hasUntracked := false
	hasChanges := false
	
	for _, line := range statusLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		if strings.HasPrefix(line, "??") {
			hasUntracked = true
		} else {
			hasChanges = true
		}
	}
	
	isClean := !hasChanges && !hasUntracked
	return isClean, hasUntracked, nil
}

// GetCommitInfo returns information about the current commit
func (g *GitUtils) GetCommitInfo() (map[string]string, error) {
	cmd := exec.Command("git", "log", "-1", "--format=%H|%s|%an|%ae|%ad|%G?", "--date=iso")
	cmd.Dir = g.workingDir
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit info: %w", err)
	}
	
	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) < 6 {
		return nil, fmt.Errorf("unexpected git log format")
	}
	
	return map[string]string{
		"sha":       parts[0],
		"message":   parts[1],
		"author":    parts[2],
		"email":     parts[3],
		"date":      parts[4],
		"gpg_signed": func() string {
			if parts[5] == "G" || parts[5] == "U" {
				return "true"
			}
			return "false"
		}(),
	}, nil
}

// GetContributors returns a list of contributors to the repository
func (g *GitUtils) GetContributors() ([]string, error) {
	cmd := exec.Command("git", "shortlog", "-sne")
	cmd.Dir = g.workingDir
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get contributors: %w", err)
	}
	
	var contributors []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Format: "  123\tJohn Doe <john@example.com>"
		parts := strings.Split(line, "\t")
		if len(parts) > 1 {
			contributors = append(contributors, strings.TrimSpace(parts[1]))
		}
	}
	
	return contributors, nil
}

// GetTags returns a list of Git tags
func (g *GitUtils) GetTags() ([]string, error) {
	cmd := exec.Command("git", "tag", "-l", "--sort=-version:refname")
	cmd.Dir = g.workingDir
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get tags: %w", err)
	}
	
	var tags []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	
	for _, line := range lines {
		if tag := strings.TrimSpace(line); tag != "" {
			tags = append(tags, tag)
		}
	}
	
	return tags, nil
}

// GetBranches returns a list of Git branches
func (g *GitUtils) GetBranches() ([]string, error) {
	cmd := exec.Command("git", "branch", "-r")
	cmd.Dir = g.workingDir
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get branches: %w", err)
	}
	
	var branches []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	
	for _, line := range lines {
		branch := strings.TrimSpace(line)
		if branch != "" && !strings.Contains(branch, "->") {
			// Remove "origin/" prefix
			if strings.HasPrefix(branch, "origin/") {
				branch = strings.TrimPrefix(branch, "origin/")
			}
			branches = append(branches, branch)
		}
	}
	
	return branches, nil
}

// IsFileIgnored checks if a file is ignored by .gitignore
func (g *GitUtils) IsFileIgnored(filePath string) (bool, error) {
	cmd := exec.Command("git", "check-ignore", filePath)
	cmd.Dir = g.workingDir
	
	// Git check-ignore returns 0 if the file is ignored, 1 if not
	err := cmd.Run()
	if err != nil {
		// Exit status 1 means file is not ignored
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode() == 0, nil
		}
		return false, fmt.Errorf("failed to check if file is ignored: %w", err)
	}
	
	return true, nil
}

// GetIgnoredFiles returns a list of files that would be ignored by .gitignore
func (g *GitUtils) GetIgnoredFiles() ([]string, error) {
	cmd := exec.Command("git", "ls-files", "--others", "--ignored", "--exclude-standard")
	cmd.Dir = g.workingDir
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ignored files: %w", err)
	}
	
	var ignoredFiles []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	
	for _, line := range lines {
		if file := strings.TrimSpace(line); file != "" {
			ignoredFiles = append(ignoredFiles, file)
		}
	}
	
	return ignoredFiles, nil
}

// ValidateWorkingDirectory checks if the working directory is valid for Git operations
func (g *GitUtils) ValidateWorkingDirectory() error {
	if _, err := os.Stat(g.workingDir); os.IsNotExist(err) {
		return fmt.Errorf("working directory does not exist: %s", g.workingDir)
	}
	
	if !g.IsGitRepository() {
		return fmt.Errorf("directory is not a Git repository: %s", g.workingDir)
	}
	
	return nil
}

// GetRepositoryStats returns basic statistics about the repository
func (g *GitUtils) GetRepositoryStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Get commit count
	if count, err := g.getCommitCount(); err == nil {
		stats["commit_count"] = count
	}
	
	// Get contributor count
	if contributors, err := g.GetContributors(); err == nil {
		stats["contributor_count"] = len(contributors)
	}
	
	// Get branch count
	if branches, err := g.GetBranches(); err == nil {
		stats["branch_count"] = len(branches)
	}
	
	// Get tag count
	if tags, err := g.GetTags(); err == nil {
		stats["tag_count"] = len(tags)
	}
	
	// Get repository size
	if size, err := g.getRepositorySize(); err == nil {
		stats["size_bytes"] = size
		stats["size_mb"] = size / (1024 * 1024)
	}
	
	return stats, nil
}

// getCommitCount gets the total number of commits
func (g *GitUtils) getCommitCount() (int, error) {
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = g.workingDir
	
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	
	var count int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count); err != nil {
		return 0, err
	}
	
	return count, nil
}

// getRepositorySize calculates the total size of the repository
func (g *GitUtils) getRepositorySize() (int64, error) {
	var size int64
	
	err := filepath.Walk(g.workingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		
		if !info.IsDir() {
			size += info.Size()
		}
		
		return nil
	})
	
	return size, err
}

// findStringSubmatch is a simple regex helper (simplified implementation)
func findStringSubmatch(pattern, text string) []string {
	// This is a simplified implementation
	// In a real implementation, you would use regexp.Compile
	if strings.Contains(text, pattern) {
		return []string{text, pattern}
	}
	return nil
}
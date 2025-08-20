package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Repository represents a Git repository with validation capabilities
type Repository struct {
	Path          string
	URL           string
	Branch        string
	SHA           string
	IsClean       bool
	HasUntracked  bool
	LastCommit    *Commit
	RemoteOrigin  *Remote
}

// Commit represents a Git commit
type Commit struct {
	SHA       string
	Message   string
	Author    string
	Email     string
	Timestamp time.Time
	GPGSigned bool
}

// Remote represents a Git remote
type Remote struct {
	Name string
	URL  string
	Type string // push, fetch
}

// ValidationResult contains the results of repository validation
type ValidationResult struct {
	Valid         bool
	Warnings      []string
	Errors        []string
	SecurityIssues []string
	Suggestions   []string
}

// RepositoryInfo provides comprehensive repository information
type RepositoryInfo struct {
	Repository    *Repository
	Validation    *ValidationResult
	Contributors  []string
	BranchCount   int
	TagCount      int
	CommitCount   int
	FirstCommit   time.Time
	LastActivity  time.Time
	Languages     map[string]int64 // language -> lines of code
}

// NewRepository creates a new repository instance from a directory
func NewRepository(path string) (*Repository, error) {
	// Validate that this is a Git repository
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", path)
	}
	
	repo := &Repository{Path: path}
	
	// Extract repository information
	if err := repo.loadRepositoryInfo(); err != nil {
		return nil, fmt.Errorf("failed to load repository info: %w", err)
	}
	
	return repo, nil
}

// loadRepositoryInfo loads comprehensive repository information
func (r *Repository) loadRepositoryInfo() error {
	var err error
	
	// Get current branch
	if r.Branch, err = r.getCurrentBranch(); err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	
	// Get current SHA
	if r.SHA, err = r.getCurrentSHA(); err != nil {
		return fmt.Errorf("failed to get current SHA: %w", err)
	}
	
	// Get repository URL
	if r.URL, err = r.getRepositoryURL(); err != nil {
		// URL extraction is not critical, continue with empty URL
		r.URL = ""
	}
	
	// Check repository status
	if r.IsClean, r.HasUntracked, err = r.getRepositoryStatus(); err != nil {
		return fmt.Errorf("failed to get repository status: %w", err)
	}
	
	// Get last commit information
	if r.LastCommit, err = r.getLastCommit(); err != nil {
		return fmt.Errorf("failed to get last commit: %w", err)
	}
	
	// Get remote origin information
	if r.RemoteOrigin, err = r.getRemoteOrigin(); err != nil {
		// Remote origin is optional, continue without error
		r.RemoteOrigin = nil
	}
	
	return nil
}

// getCurrentBranch gets the current Git branch
func (r *Repository) getCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = r.Path
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	
	branch := strings.TrimSpace(string(output))
	if branch == "HEAD" {
		// We're in a detached HEAD state, try to get the SHA instead
		return "detached", nil
	}
	
	return branch, nil
}

// getCurrentSHA gets the current Git SHA
func (r *Repository) getCurrentSHA() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = r.Path
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current SHA: %w", err)
	}
	
	return strings.TrimSpace(string(output)), nil
}

// getShortSHA gets the short version of the current SHA
func (r *Repository) GetShortSHA() string {
	if len(r.SHA) >= 12 {
		return r.SHA[:12]
	}
	return r.SHA
}

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
	if strings.HasSuffix(url, ".git") {
		url = url[:len(url)-4]
	}
	
	return url
}

// getRepositoryStatus checks if the repository is clean and has untracked files
func (r *Repository) getRepositoryStatus() (bool, bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.Path
	
	output, err := cmd.Output()
	if err != nil {
		return false, false, fmt.Errorf("failed to get repository status: %w", err)
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
	
	return !hasChanges && !hasUntracked, hasUntracked, nil
}

// getLastCommit retrieves information about the last commit
func (r *Repository) getLastCommit() (*Commit, error) {
	// Get commit information with format
	cmd := exec.Command("git", "log", "-1", "--format=%H|%s|%an|%ae|%ct|%G?")
	cmd.Dir = r.Path
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get last commit: %w", err)
	}
	
	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) < 6 {
		return nil, fmt.Errorf("unexpected git log format")
	}
	
	// Parse timestamp
	timestamp := time.Unix(0, 0)
	if ts, err := time.Parse("1136214245", parts[4]); err == nil {
		timestamp = ts
	}
	
	// Check GPG signature
	gpgSigned := parts[5] == "G" || parts[5] == "U"
	
	return &Commit{
		SHA:       parts[0],
		Message:   parts[1],
		Author:    parts[2],
		Email:     parts[3],
		Timestamp: timestamp,
		GPGSigned: gpgSigned,
	}, nil
}

// getRemoteOrigin gets information about the origin remote
func (r *Repository) getRemoteOrigin() (*Remote, error) {
	cmd := exec.Command("git", "remote", "-v")
	cmd.Dir = r.Path
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get remotes: %w", err)
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "origin") && strings.Contains(line, "fetch") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return &Remote{
					Name: parts[0],
					URL:  parts[1],
					Type: strings.Trim(parts[2], "()"),
				}, nil
			}
		}
	}
	
	return nil, fmt.Errorf("no origin remote found")
}

// ValidateRepository performs comprehensive repository validation
func (r *Repository) ValidateRepository() *ValidationResult {
	result := &ValidationResult{
		Valid:         true,
		Warnings:      []string{},
		Errors:        []string{},
		SecurityIssues: []string{},
		Suggestions:   []string{},
	}
	
	// Validate repository URL
	r.validateURL(result)
	
	// Validate commit signature
	r.validateCommitSigning(result)
	
	// Validate repository cleanliness
	r.validateCleanliness(result)
	
	// Validate branch status
	r.validateBranch(result)
	
	// Security validations
	r.validateSecurity(result)
	
	// Set overall validity
	result.Valid = len(result.Errors) == 0
	
	return result
}

// validateURL validates the repository URL
func (r *Repository) validateURL(result *ValidationResult) {
	if r.URL == "" {
		result.Warnings = append(result.Warnings, "No repository URL found")
		return
	}
	
	// Check for trusted domains
	trustedDomains := []string{"github.com", "gitlab.com", "bitbucket.org"}
	isTrusted := false
	
	for _, domain := range trustedDomains {
		if strings.Contains(r.URL, domain) {
			isTrusted = true
			break
		}
	}
	
	if !isTrusted {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Repository URL is not from a trusted domain: %s", r.URL))
	}
	
	// Check for HTTPS
	if !strings.HasPrefix(r.URL, "https://") {
		result.SecurityIssues = append(result.SecurityIssues, "Repository URL should use HTTPS")
	}
}

// validateCommitSigning validates commit signing
func (r *Repository) validateCommitSigning(result *ValidationResult) {
	if r.LastCommit == nil {
		result.Errors = append(result.Errors, "No commit information available")
		return
	}
	
	if !r.LastCommit.GPGSigned {
		result.Warnings = append(result.Warnings, "Last commit is not GPG signed")
		result.Suggestions = append(result.Suggestions, "Enable GPG signing with: git config --global commit.gpgsign true")
	}
}

// validateCleanliness validates repository cleanliness
func (r *Repository) validateCleanliness(result *ValidationResult) {
	if !r.IsClean {
		result.Warnings = append(result.Warnings, "Repository has uncommitted changes")
		result.Suggestions = append(result.Suggestions, "Commit or stash changes before deployment")
	}
	
	if r.HasUntracked {
		result.Warnings = append(result.Warnings, "Repository has untracked files")
		result.Suggestions = append(result.Suggestions, "Add untracked files to .gitignore or commit them")
	}
}

// validateBranch validates branch status
func (r *Repository) validateBranch(result *ValidationResult) {
	if r.Branch == "detached" {
		result.Warnings = append(result.Warnings, "Repository is in detached HEAD state")
		result.Suggestions = append(result.Suggestions, "Switch to a named branch: git checkout -b <branch-name>")
	}
	
	// Check if on default branch
	defaultBranches := []string{"main", "master"}
	isDefaultBranch := false
	for _, branch := range defaultBranches {
		if r.Branch == branch {
			isDefaultBranch = true
			break
		}
	}
	
	if !isDefaultBranch {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Not on default branch (current: %s)", r.Branch))
	}
}

// validateSecurity performs security-related validations
func (r *Repository) validateSecurity(result *ValidationResult) {
	// Check for common security issues in repository
	r.checkForSecrets(result)
	r.checkForSensitiveFiles(result)
}

// checkForSecrets checks for potential secrets in repository
func (r *Repository) checkForSecrets(result *ValidationResult) {
	secretPatterns := []struct {
		name    string
		pattern *regexp.Regexp
	}{
		{"AWS Access Key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
		{"Private Key", regexp.MustCompile(`-----BEGIN.*PRIVATE KEY-----`)},
		{"API Key", regexp.MustCompile(`(?i)api[_-]?key[^a-zA-Z0-9]`)},
		{"Password", regexp.MustCompile(`(?i)password[^a-zA-Z0-9]`)},
		{"Token", regexp.MustCompile(`(?i)token[^a-zA-Z0-9]`)},
	}
	
	// Check files for secret patterns
	err := filepath.Walk(r.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		
		// Skip binary files and .git directory
		if info.IsDir() || strings.Contains(path, ".git") {
			return nil
		}
		
		// Only check text files
		if !r.isTextFile(path) {
			return nil
		}
		
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		
		content := string(data)
		relativePath, _ := filepath.Rel(r.Path, path)
		
		for _, secret := range secretPatterns {
			if secret.pattern.MatchString(content) {
				result.SecurityIssues = append(result.SecurityIssues, 
					fmt.Sprintf("Potential %s found in %s", secret.name, relativePath))
			}
		}
		
		return nil
	})
	
	if err != nil {
		result.Warnings = append(result.Warnings, "Failed to scan for secrets")
	}
}

// checkForSensitiveFiles checks for sensitive files that shouldn't be committed
func (r *Repository) checkForSensitiveFiles(result *ValidationResult) {
	sensitiveFiles := []string{
		".env",
		".env.local",
		".env.production",
		"secrets.yaml",
		"secrets.json",
		"private.key",
		"*.pem",
		"id_rsa",
		"id_dsa",
		"*.p12",
		"*.pfx",
	}
	
	for _, pattern := range sensitiveFiles {
		matches, _ := filepath.Glob(filepath.Join(r.Path, pattern))
		for _, match := range matches {
			relativePath, _ := filepath.Rel(r.Path, match)
			result.SecurityIssues = append(result.SecurityIssues, 
				fmt.Sprintf("Sensitive file committed: %s", relativePath))
		}
	}
}

// isTextFile checks if a file is likely a text file
func (r *Repository) isTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	textExtensions := []string{
		".txt", ".md", ".go", ".js", ".ts", ".py", ".java", ".c", ".cpp",
		".h", ".hpp", ".rs", ".rb", ".php", ".html", ".css", ".scss",
		".yaml", ".yml", ".json", ".xml", ".toml", ".ini", ".conf",
		".sh", ".bash", ".fish", ".ps1", ".dockerfile", ".makefile",
	}
	
	for _, textExt := range textExtensions {
		if ext == textExt {
			return true
		}
	}
	
	return false
}

// GetRepositoryInfo returns comprehensive repository information
func (r *Repository) GetRepositoryInfo() (*RepositoryInfo, error) {
	info := &RepositoryInfo{
		Repository: r,
		Validation: r.ValidateRepository(),
		Languages:  make(map[string]int64),
	}
	
	// Get contributor count
	contributors, err := r.getContributors()
	if err == nil {
		info.Contributors = contributors
	}
	
	// Get branch and tag counts
	info.BranchCount, _ = r.getBranchCount()
	info.TagCount, _ = r.getTagCount()
	
	// Get commit count
	info.CommitCount, _ = r.getCommitCount()
	
	// Get first commit and last activity
	info.FirstCommit, _ = r.getFirstCommitTime()
	info.LastActivity, _ = r.getLastActivityTime()
	
	// Get language statistics
	info.Languages, _ = r.getLanguageStats()
	
	return info, nil
}

// getContributors gets list of contributors
func (r *Repository) getContributors() ([]string, error) {
	cmd := exec.Command("git", "shortlog", "-sne")
	cmd.Dir = r.Path
	
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	var contributors []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Extract email from format: "  123\tJohn Doe <john@example.com>"
		parts := strings.Split(line, "\t")
		if len(parts) > 1 {
			contributors = append(contributors, strings.TrimSpace(parts[1]))
		}
	}
	
	return contributors, nil
}

// getBranchCount gets the number of branches
func (r *Repository) getBranchCount() (int, error) {
	cmd := exec.Command("git", "branch", "-r")
	cmd.Dir = r.Path
	
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" && !strings.Contains(line, "->") {
			count++
		}
	}
	
	return count, nil
}

// getTagCount gets the number of tags
func (r *Repository) getTagCount() (int, error) {
	cmd := exec.Command("git", "tag", "--list")
	cmd.Dir = r.Path
	
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	
	return count, nil
}

// getCommitCount gets the total number of commits
func (r *Repository) getCommitCount() (int, error) {
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = r.Path
	
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	
	count := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count); err != nil {
		return 0, err
	}
	
	return count, nil
}

// getFirstCommitTime gets the time of the first commit
func (r *Repository) getFirstCommitTime() (time.Time, error) {
	cmd := exec.Command("git", "log", "--reverse", "--format=%ct", "-1")
	cmd.Dir = r.Path
	
	output, err := cmd.Output()
	if err != nil {
		return time.Time{}, err
	}
	
	timestamp := strings.TrimSpace(string(output))
	if ts, err := time.Parse("1136214245", timestamp); err == nil {
		return ts, nil
	}
	
	return time.Time{}, fmt.Errorf("failed to parse first commit timestamp")
}

// getLastActivityTime gets the time of the last activity
func (r *Repository) getLastActivityTime() (time.Time, error) {
	if r.LastCommit != nil {
		return r.LastCommit.Timestamp, nil
	}
	return time.Time{}, fmt.Errorf("no last commit information")
}

// getLanguageStats gets basic language statistics
func (r *Repository) getLanguageStats() (map[string]int64, error) {
	languages := make(map[string]int64)
	
	err := filepath.Walk(r.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		
		if info.IsDir() || strings.Contains(path, ".git") {
			return nil
		}
		
		ext := strings.ToLower(filepath.Ext(path))
		if ext == "" {
			return nil
		}
		
		// Map extensions to languages
		langMap := map[string]string{
			".go":   "Go",
			".js":   "JavaScript",
			".ts":   "TypeScript",
			".py":   "Python",
			".java": "Java",
			".c":    "C",
			".cpp":  "C++",
			".rs":   "Rust",
			".rb":   "Ruby",
			".php":  "PHP",
		}
		
		if lang, exists := langMap[ext]; exists {
			languages[lang] += info.Size()
		}
		
		return nil
	})
	
	return languages, err
}
package git

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

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

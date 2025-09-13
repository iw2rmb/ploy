package git

import (
	"os"
	"path/filepath"
	"strings"
)

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

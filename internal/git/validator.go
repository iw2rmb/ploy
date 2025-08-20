package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidationLevel represents the strictness of Git validation
type ValidationLevel int

const (
	ValidationLevelNone ValidationLevel = iota
	ValidationLevelWarning
	ValidationLevelStrict
)

// ValidatorConfig configures Git validation behavior
type ValidatorConfig struct {
	Level                ValidationLevel
	RequireCleanRepo     bool
	RequireSignedCommits bool
	RequireTrustedOrigin bool
	AllowedBranches      []string
	TrustedDomains       []string
	MaxRepoSizeMB        int64
	ScanForSecrets       bool
}

// DefaultValidatorConfig returns a default validation configuration
func DefaultValidatorConfig() *ValidatorConfig {
	return &ValidatorConfig{
		Level:                ValidationLevelWarning,
		RequireCleanRepo:     false,
		RequireSignedCommits: false,
		RequireTrustedOrigin: false,
		AllowedBranches:      []string{"main", "master", "develop", "staging", "production"},
		TrustedDomains:       []string{"github.com", "gitlab.com", "bitbucket.org"},
		MaxRepoSizeMB:        500, // 500MB max repository size
		ScanForSecrets:       true,
	}
}

// ProductionValidatorConfig returns a strict validation configuration for production
func ProductionValidatorConfig() *ValidatorConfig {
	return &ValidatorConfig{
		Level:                ValidationLevelStrict,
		RequireCleanRepo:     true,
		RequireSignedCommits: true,
		RequireTrustedOrigin: true,
		AllowedBranches:      []string{"main", "master", "production"},
		TrustedDomains:       []string{"github.com", "gitlab.com"},
		MaxRepoSizeMB:        100, // Stricter size limit for production
		ScanForSecrets:       true,
	}
}

// Validator performs Git repository validation with configurable rules
type Validator struct {
	config *ValidatorConfig
}

// NewValidator creates a new Git validator with the specified configuration
func NewValidator(config *ValidatorConfig) *Validator {
	if config == nil {
		config = DefaultValidatorConfig()
	}
	return &Validator{config: config}
}

// ValidateRepository performs comprehensive validation of a Git repository
func (v *Validator) ValidateRepository(repoPath string) (*ValidationResult, error) {
	// Create repository instance
	repo, err := NewRepository(repoPath)
	if err != nil {
		return &ValidationResult{
			Valid: false,
			Errors: []string{fmt.Sprintf("Failed to initialize repository: %v", err)},
		}, err
	}
	
	// Perform basic validation
	result := repo.ValidateRepository()
	
	// Apply configuration-specific validation
	v.applyConfigValidation(repo, result)
	
	// Final validation based on level
	v.finalizeValidation(result)
	
	return result, nil
}

// applyConfigValidation applies configuration-specific validation rules
func (v *Validator) applyConfigValidation(repo *Repository, result *ValidationResult) {
	// Validate repository cleanliness
	if v.config.RequireCleanRepo {
		if !repo.IsClean {
			result.Errors = append(result.Errors, "Repository must be clean (no uncommitted changes)")
		}
		if repo.HasUntracked {
			result.Errors = append(result.Errors, "Repository must not have untracked files")
		}
	}
	
	// Validate commit signing
	if v.config.RequireSignedCommits && repo.LastCommit != nil {
		if !repo.LastCommit.GPGSigned {
			result.Errors = append(result.Errors, "Last commit must be GPG signed")
		}
	}
	
	// Validate repository origin
	if v.config.RequireTrustedOrigin {
		if repo.URL == "" {
			result.Errors = append(result.Errors, "Repository must have a remote origin URL")
		} else {
			isTrusted := false
			for _, domain := range v.config.TrustedDomains {
				if strings.Contains(repo.URL, domain) {
					isTrusted = true
					break
				}
			}
			if !isTrusted {
				result.Errors = append(result.Errors, fmt.Sprintf("Repository origin must be from trusted domains: %v", v.config.TrustedDomains))
			}
		}
	}
	
	// Validate branch
	if len(v.config.AllowedBranches) > 0 {
		branchAllowed := false
		for _, allowed := range v.config.AllowedBranches {
			if repo.Branch == allowed {
				branchAllowed = true
				break
			}
		}
		if !branchAllowed {
			if v.config.Level == ValidationLevelStrict {
				result.Errors = append(result.Errors, fmt.Sprintf("Branch '%s' not allowed. Allowed branches: %v", repo.Branch, v.config.AllowedBranches))
			} else {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Branch '%s' not in recommended branches: %v", repo.Branch, v.config.AllowedBranches))
			}
		}
	}
	
	// Validate repository size
	if v.config.MaxRepoSizeMB > 0 {
		if repoSize, err := v.getRepositorySize(repo.Path); err == nil {
			repoSizeMB := repoSize / (1024 * 1024)
			if repoSizeMB > v.config.MaxRepoSizeMB {
				if v.config.Level == ValidationLevelStrict {
					result.Errors = append(result.Errors, fmt.Sprintf("Repository size (%d MB) exceeds maximum allowed size (%d MB)", repoSizeMB, v.config.MaxRepoSizeMB))
				} else {
					result.Warnings = append(result.Warnings, fmt.Sprintf("Repository size (%d MB) is large (max recommended: %d MB)", repoSizeMB, v.config.MaxRepoSizeMB))
				}
			}
		}
	}
}

// finalizeValidation finalizes validation based on configuration level
func (v *Validator) finalizeValidation(result *ValidationResult) {
	switch v.config.Level {
	case ValidationLevelNone:
		// Clear all errors and warnings, only keep security issues
		result.Errors = []string{}
		result.Warnings = []string{}
		result.Valid = len(result.SecurityIssues) == 0
		
	case ValidationLevelWarning:
		// Errors make it invalid, warnings and security issues are just reported
		result.Valid = len(result.Errors) == 0
		
	case ValidationLevelStrict:
		// Any issue makes it invalid
		result.Valid = len(result.Errors) == 0 && len(result.SecurityIssues) == 0
		// Promote security issues to errors
		for _, issue := range result.SecurityIssues {
			result.Errors = append(result.Errors, fmt.Sprintf("Security issue: %s", issue))
		}
	}
}

// getRepositorySize calculates the total size of a repository
func (v *Validator) getRepositorySize(repoPath string) (int64, error) {
	var size int64
	
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
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

// ValidateForEnvironment validates a repository for a specific environment
func (v *Validator) ValidateForEnvironment(repoPath string, environment string) (*ValidationResult, error) {
	// Adjust configuration based on environment
	originalConfig := v.config
	
	switch strings.ToLower(environment) {
	case "production", "prod", "live":
		v.config = ProductionValidatorConfig()
	case "staging", "stage":
		config := DefaultValidatorConfig()
		config.Level = ValidationLevelStrict
		config.RequireCleanRepo = true
		v.config = config
	case "development", "dev":
		v.config = DefaultValidatorConfig()
		v.config.Level = ValidationLevelWarning
	default:
		// Use current configuration
	}
	
	// Perform validation
	result, err := v.ValidateRepository(repoPath)
	
	// Restore original configuration
	v.config = originalConfig
	
	return result, err
}

// GetRepositorySummary returns a human-readable summary of repository validation
func (v *Validator) GetRepositorySummary(repoPath string) (string, error) {
	repo, err := NewRepository(repoPath)
	if err != nil {
		return "", err
	}
	
	info, err := repo.GetRepositoryInfo()
	if err != nil {
		return "", err
	}
	
	var summary strings.Builder
	
	// Repository basic info
	summary.WriteString(fmt.Sprintf("Repository: %s\n", repo.URL))
	summary.WriteString(fmt.Sprintf("Branch: %s\n", repo.Branch))
	summary.WriteString(fmt.Sprintf("SHA: %s\n", repo.GetShortSHA()))
	summary.WriteString(fmt.Sprintf("Clean: %t\n", repo.IsClean))
	
	// Commit information
	if repo.LastCommit != nil {
		summary.WriteString(fmt.Sprintf("Last Commit: %s\n", repo.LastCommit.Message))
		summary.WriteString(fmt.Sprintf("Author: %s <%s>\n", repo.LastCommit.Author, repo.LastCommit.Email))
		summary.WriteString(fmt.Sprintf("GPG Signed: %t\n", repo.LastCommit.GPGSigned))
	}
	
	// Repository statistics
	summary.WriteString(fmt.Sprintf("Contributors: %d\n", len(info.Contributors)))
	summary.WriteString(fmt.Sprintf("Branches: %d\n", info.BranchCount))
	summary.WriteString(fmt.Sprintf("Tags: %d\n", info.TagCount))
	summary.WriteString(fmt.Sprintf("Commits: %d\n", info.CommitCount))
	
	// Validation results
	result := info.Validation
	summary.WriteString(fmt.Sprintf("\nValidation: %t\n", result.Valid))
	
	if len(result.Errors) > 0 {
		summary.WriteString(fmt.Sprintf("Errors (%d):\n", len(result.Errors)))
		for _, err := range result.Errors {
			summary.WriteString(fmt.Sprintf("  - %s\n", err))
		}
	}
	
	if len(result.Warnings) > 0 {
		summary.WriteString(fmt.Sprintf("Warnings (%d):\n", len(result.Warnings)))
		for _, warning := range result.Warnings {
			summary.WriteString(fmt.Sprintf("  - %s\n", warning))
		}
	}
	
	if len(result.SecurityIssues) > 0 {
		summary.WriteString(fmt.Sprintf("Security Issues (%d):\n", len(result.SecurityIssues)))
		for _, issue := range result.SecurityIssues {
			summary.WriteString(fmt.Sprintf("  - %s\n", issue))
		}
	}
	
	if len(result.Suggestions) > 0 {
		summary.WriteString(fmt.Sprintf("Suggestions (%d):\n", len(result.Suggestions)))
		for _, suggestion := range result.Suggestions {
			summary.WriteString(fmt.Sprintf("  - %s\n", suggestion))
		}
	}
	
	return summary.String(), nil
}

// IsRepositoryValid performs a quick validation check
func (v *Validator) IsRepositoryValid(repoPath string) bool {
	result, err := v.ValidateRepository(repoPath)
	if err != nil {
		return false
	}
	return result.Valid
}

// GetRepositoryHealth returns a health score (0-100) for the repository
func (v *Validator) GetRepositoryHealth(repoPath string) (int, error) {
	result, err := v.ValidateRepository(repoPath)
	if err != nil {
		return 0, err
	}
	
	score := 100
	
	// Deduct points for issues
	score -= len(result.Errors) * 20      // Major issues
	score -= len(result.SecurityIssues) * 15  // Security issues
	score -= len(result.Warnings) * 5    // Minor issues
	
	// Ensure score doesn't go below 0
	if score < 0 {
		score = 0
	}
	
	return score, nil
}
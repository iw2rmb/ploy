package git

import (
	"fmt"
	"strings"
)

// ValidateRepository performs comprehensive repository validation
func (r *Repository) ValidateRepository() *ValidationResult {
	result := &ValidationResult{
		Valid:          true,
		Warnings:       []string{},
		Errors:         []string{},
		SecurityIssues: []string{},
		Suggestions:    []string{},
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

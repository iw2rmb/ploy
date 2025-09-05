package provider

import "context"

// GitProvider defines the interface for git forge providers (GitLab, GitHub, etc.)
type GitProvider interface {
	CreateOrUpdateMR(ctx context.Context, config MRConfig) (*MRResult, error)
	ValidateConfiguration() error
}

// MRConfig contains all parameters needed to create or update a merge request
type MRConfig struct {
	RepoURL      string   // HTTPS repository URL (e.g., "https://gitlab.example.com/namespace/project.git")
	SourceBranch string   // workflow branch name (e.g., "workflow/java17-migration/20250905")
	TargetBranch string   // base_ref from config (e.g., "refs/heads/main")
	Title        string   // MR title derived from workflow
	Description  string   // MR description with step summaries
	Labels       []string // default labels ["ploy", "tfl"]
}

// MRResult contains the result of MR creation or update
type MRResult struct {
	MRURL   string // GitLab MR web URL for browser access
	MRID    int    // GitLab MR numeric ID for API operations
	Created bool   // true if created, false if updated existing MR
}

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// GitLabProvider implements GitProvider for GitLab instances
type GitLabProvider struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewGitLabProvider creates a new GitLab provider instance
func NewGitLabProvider() *GitLabProvider {
	baseURL := os.Getenv("GITLAB_URL")
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}

	token := os.Getenv("PLOY_GITLAB_PAT")

	return &GitLabProvider{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{},
	}
}

// ValidateConfiguration validates that required configuration is present
func (g *GitLabProvider) ValidateConfiguration() error {
	if g.token == "" {
		return fmt.Errorf("PLOY_GITLAB_PAT environment variable is required")
	}
	return nil
}

// CreateOrUpdateMR creates or updates a GitLab merge request
func (g *GitLabProvider) CreateOrUpdateMR(ctx context.Context, config MRConfig) (*MRResult, error) {
	if err := g.ValidateConfiguration(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Extract GitLab project from repository URL
	project, err := extractGitLabProject(config.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract GitLab project from URL %s: %w", config.RepoURL, err)
	}

	// Check for existing MR with same source branch
	existingMR, err := g.findExistingMR(ctx, project, config.SourceBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing MR: %w", err)
	}

	if existingMR != nil {
		// Update existing MR
		result, err := g.updateMR(ctx, project, existingMR.IID, config)
		if err != nil {
			return nil, fmt.Errorf("failed to update existing MR: %w", err)
		}
		result.Created = false
		return result, nil
	}

	// Create new MR
	result, err := g.createMR(ctx, project, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create new MR: %w", err)
	}
	result.Created = true
	return result, nil
}

// findExistingMR searches for an existing MR with the same source branch
func (g *GitLabProvider) findExistingMR(ctx context.Context, project, sourceBranch string) (*gitLabMRResponse, error) {
	// URL encode the project path
	encodedProject := url.PathEscape(project)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests?source_branch=%s&state=opened",
		g.baseURL, encodedProject, url.QueryEscape(sourceBranch))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make API request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API error: status %d", resp.StatusCode)
	}

	var mrs []gitLabMRResponse
	if err := json.NewDecoder(resp.Body).Decode(&mrs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(mrs) > 0 {
		return &mrs[0], nil
	}

	return nil, nil // No existing MR found
}

// createMR creates a new GitLab merge request
func (g *GitLabProvider) createMR(ctx context.Context, project string, config MRConfig) (*MRResult, error) {
	// Prepare target branch (strip refs/heads/ prefix if present)
	targetBranch := strings.TrimPrefix(config.TargetBranch, "refs/heads/")

	mrRequest := gitLabMRRequest{
		Title:        config.Title,
		Description:  config.Description,
		SourceBranch: config.SourceBranch,
		TargetBranch: targetBranch,
		Labels:       strings.Join(config.Labels, ","),
	}

	body, err := json.Marshal(mrRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	encodedProject := url.PathEscape(project)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests", g.baseURL, encodedProject)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make API request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("GitLab API error: status %d", resp.StatusCode)
	}

	var mrResponse gitLabMRResponse
	if err := json.NewDecoder(resp.Body).Decode(&mrResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &MRResult{
		MRURL: mrResponse.WebURL,
		MRID:  mrResponse.IID,
	}, nil
}

// updateMR updates an existing GitLab merge request
func (g *GitLabProvider) updateMR(ctx context.Context, project string, mrIID int, config MRConfig) (*MRResult, error) {
	// Prepare target branch (strip refs/heads/ prefix if present)
	targetBranch := strings.TrimPrefix(config.TargetBranch, "refs/heads/")

	mrRequest := gitLabMRRequest{
		Title:        config.Title,
		Description:  config.Description,
		TargetBranch: targetBranch,
		Labels:       strings.Join(config.Labels, ","),
	}

	body, err := json.Marshal(mrRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	encodedProject := url.PathEscape(project)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d", g.baseURL, encodedProject, mrIID)

	req, err := http.NewRequestWithContext(ctx, "PUT", apiURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make API request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API error: status %d", resp.StatusCode)
	}

	var mrResponse gitLabMRResponse
	if err := json.NewDecoder(resp.Body).Decode(&mrResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &MRResult{
		MRURL: mrResponse.WebURL,
		MRID:  mrResponse.IID,
	}, nil
}

// extractGitLabProject extracts the project namespace/name from a GitLab repository URL
func extractGitLabProject(repoURL string) (string, error) {
	parsedURL, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}

	// Validate that this is an HTTPS URL
	if parsedURL.Scheme != "https" {
		return "", fmt.Errorf("repository URL must use HTTPS scheme, got: %s", parsedURL.Scheme)
	}

	// Validate that host is set (basic validation for proper URL)
	if parsedURL.Host == "" {
		return "", fmt.Errorf("repository URL must have a valid host")
	}

	// Remove leading slash and .git suffix
	path := strings.TrimPrefix(parsedURL.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	if path == "" {
		return "", fmt.Errorf("unable to extract project path from URL: %s", repoURL)
	}

	// Basic validation - should have at least namespace/project
	pathParts := strings.Split(path, "/")
	if len(pathParts) < 2 {
		return "", fmt.Errorf("GitLab project path should have at least namespace/project format, got: %s", path)
	}

	return path, nil
}

// GitLab API data structures

// gitLabMRRequest represents a GitLab merge request creation/update request
type gitLabMRRequest struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	SourceBranch string `json:"source_branch,omitempty"`
	TargetBranch string `json:"target_branch"`
	Labels       string `json:"labels,omitempty"`
}

// gitLabMRResponse represents a GitLab merge request API response
type gitLabMRResponse struct {
	ID           int    `json:"id"`
	IID          int    `json:"iid"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	State        string `json:"state"`
	WebURL       string `json:"web_url"`
}

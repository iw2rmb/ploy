package gitlab

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	gitlabapi "gitlab.com/gitlab-org/api/client-go"
)

// MRClient provides GitLab merge request creation functionality.
// It uses the GitLab client-go library for typed API interactions.
type MRClient struct {
	httpClient *http.Client
}

// NewMRClient creates a new GitLab MR client.
// The HTTP client will be used to configure the GitLab API client for each request.
func NewMRClient() *MRClient {
	return &MRClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// MRCreateRequest holds parameters for creating a merge request.
type MRCreateRequest struct {
	// Domain is the GitLab domain (e.g., "gitlab.com" or "gitlab.example.com").
	Domain string
	// ProjectID is the URL-encoded project path (e.g., "org%2Fproject").
	ProjectID string
	// PAT is the Personal Access Token for authentication.
	PAT string
	// Title is the MR title.
	Title string
	// SourceBranch is the branch containing changes.
	SourceBranch string
	// TargetBranch is the branch to merge into (usually "main" or "master").
	TargetBranch string
	// Description is the MR description (optional).
	Description string
	// Labels is a comma-separated list of labels (optional).
	Labels string
}

// CreateMR creates a merge request in GitLab using the provided parameters.
// Returns the MR URL on success.
// Retries on 429 (rate limit) and 5xx (server errors) with exponential backoff (max 4 attempts).
func (c *MRClient) CreateMR(ctx context.Context, req MRCreateRequest) (string, error) {
	if err := validateMRCreateRequest(req); err != nil {
		return "", git.RedactError(fmt.Errorf("invalid request: %w", err), req.PAT)
	}

	// Create GitLab API client using the shared configuration helper.
	// This client is configured with the appropriate base URL (http/https based on domain),
	// and dual auth headers (Authorization + PRIVATE-TOKEN) for compatibility.
	glClient, err := NewClient(ClientConfig{
		Domain:     req.Domain,
		PAT:        req.PAT,
		HTTPClient: c.httpClient,
	})
	if err != nil {
		return "", git.RedactError(fmt.Errorf("create gitlab client: %w", err), req.PAT)
	}

	// Decode the URL-encoded project ID since the client-go library will re-encode it.
	// Our external contract uses URL-encoded project IDs (e.g., "org%2Fproject"),
	// but the library expects unencoded strings (e.g., "org/project") and handles encoding internally.
	projectID, err := url.PathUnescape(req.ProjectID)
	if err != nil {
		return "", git.RedactError(fmt.Errorf("invalid project_id: %w", err), req.PAT)
	}

	// Build merge request creation options using client-go types.
	// The library uses pointer fields for optional parameters.
	options := &gitlabapi.CreateMergeRequestOptions{
		Title:        &req.Title,
		SourceBranch: &req.SourceBranch,
		TargetBranch: &req.TargetBranch,
	}

	// Add optional description if provided (non-empty after trimming).
	if desc := strings.TrimSpace(req.Description); desc != "" {
		options.Description = &desc
	}

	// Add optional labels if provided (non-empty after trimming).
	// The client-go library expects a LabelOptions (slice of strings) that will be
	// marshaled as a comma-separated string in the JSON request.
	if labels := strings.TrimSpace(req.Labels); labels != "" {
		// Split comma-separated labels into a slice.
		labelSlice := strings.Split(labels, ",")
		for i := range labelSlice {
			labelSlice[i] = strings.TrimSpace(labelSlice[i])
		}
		labelOpts := gitlabapi.LabelOptions(labelSlice)
		options.Labels = &labelOpts
	}

	// Result holder for successful response web URL.
	var webURL string

	// Retry operation using shared backoff helper with GitLabMRPolicy.
	// The policy provides 4 max attempts (initial + 3 retries) with 1s initial interval
	// and 2x multiplier (1s, 2s, 4s backoff schedule), plus 50% jitter for robustness.
	// The helper honors context cancellation and logs each attempt via slog.
	policy := backoff.GitLabMRPolicy()
	err = backoff.RunWithBackoff(ctx, policy, slog.Default(), func() error {
		// Call the GitLab API to create the merge request.
		// The client-go library handles JSON marshaling and HTTP request construction.
		mr, resp, err := glClient.MergeRequests.CreateMergeRequest(projectID, options)

		// Handle network or API errors.
		if err != nil {
			// Check if context was cancelled (don't retry).
			if ctx.Err() != nil {
				return backoff.Permanent(err)
			}

			// Check if the error is retryable based on HTTP response status code.
			// Retry on 429 (rate limit) and 5xx (server errors); network errors are retryable by default.
			if resp != nil {
				statusCode := resp.StatusCode
				// Retry on 429 (Too Many Requests) or 5xx (server errors).
				if statusCode == http.StatusTooManyRequests || (statusCode >= 500 && statusCode < 600) {
					return fmt.Errorf("gitlab api error (retryable): %w", err)
				}
				// Non-retryable HTTP error (e.g., 4xx client errors like 400, 401, 404).
				return backoff.Permanent(fmt.Errorf("create merge request: %w", err))
			}

			// Network error without HTTP response (e.g., connection refused, DNS failure).
			// These are transient failures that should be retried.
			return fmt.Errorf("gitlab api network error: %w", err)
		}

		// Verify that the response includes the web URL.
		if mr == nil || mr.WebURL == "" {
			return backoff.Permanent(fmt.Errorf("no web_url in merge request response"))
		}

		// Store the result and return success.
		webURL = mr.WebURL
		return nil
	})

	if err != nil {
		return "", git.RedactError(err, req.PAT)
	}

	return webURL, nil
}

// validateMRCreateRequest checks that required fields are provided.
func validateMRCreateRequest(req MRCreateRequest) error {
	if strings.TrimSpace(req.Domain) == "" {
		return fmt.Errorf("domain is required")
	}
	if strings.TrimSpace(req.ProjectID) == "" {
		return fmt.Errorf("project_id is required")
	}
	if strings.TrimSpace(req.PAT) == "" {
		return fmt.Errorf("pat is required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(req.SourceBranch) == "" {
		return fmt.Errorf("source_branch is required")
	}
	if strings.TrimSpace(req.TargetBranch) == "" {
		return fmt.Errorf("target_branch is required")
	}
	return nil
}

// ExtractProjectIDFromURL extracts the URL-encoded project ID from a GitLab HTTPS URL.
// For example: "https://gitlab.com/org/project.git" -> "org%2Fproject".
func ExtractProjectIDFromURL(repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("parse repo url: %w", err)
	}

	// Validate that we have a proper URL with a scheme and host.
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid repo url: missing scheme or host")
	}

	// Extract path and trim leading "/" and trailing ".git".
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	if path == "" {
		return "", fmt.Errorf("empty project path")
	}

	// URL-encode the path for GitLab API.
	return url.PathEscape(path), nil
}

package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MRClient provides GitLab merge request creation functionality.
type MRClient struct {
	client *http.Client
}

// NewMRClient creates a new GitLab MR client.
func NewMRClient() *MRClient {
	return &MRClient{
		client: &http.Client{
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

// MRCreateResponse holds the response from creating a merge request.
type MRCreateResponse struct {
	// WebURL is the URL to view the MR in GitLab.
	WebURL string
	// IID is the internal ID of the MR within the project.
	IID int
}

// CreateMR creates a merge request in GitLab using the provided parameters.
// Returns the MR URL on success.
func (c *MRClient) CreateMR(ctx context.Context, req MRCreateRequest) (string, error) {
	if err := validateMRCreateRequest(req); err != nil {
		return "", redactError(fmt.Errorf("invalid request: %w", err), req.PAT)
	}

	// Construct GitLab API URL.
	// Use http:// scheme if domain starts with localhost or 127.0.0.1 (for testing).
	scheme := "https"
	if strings.HasPrefix(req.Domain, "localhost") || strings.HasPrefix(req.Domain, "127.0.0.1") {
		scheme = "http"
	}
	apiURL := fmt.Sprintf("%s://%s/api/v4/projects/%s/merge_requests",
		scheme, req.Domain, req.ProjectID)

	// Build request payload.
	payload := map[string]interface{}{
		"title":         req.Title,
		"source_branch": req.SourceBranch,
		"target_branch": req.TargetBranch,
	}

	if strings.TrimSpace(req.Description) != "" {
		payload["description"] = req.Description
	}

	if strings.TrimSpace(req.Labels) != "" {
		payload["labels"] = req.Labels
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", redactError(fmt.Errorf("marshal request: %w", err), req.PAT)
	}

	// Create HTTP request.
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", redactError(fmt.Errorf("create request: %w", err), req.PAT)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.PAT)

	// Send request.
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", redactError(fmt.Errorf("send request: %w", err), req.PAT)
	}
	defer resp.Body.Close()

	// Read response body.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", redactError(fmt.Errorf("read response: %w", err), req.PAT)
	}

	// Check response status.
	if resp.StatusCode != http.StatusCreated {
		return "", redactError(
			fmt.Errorf("gitlab api error: status %d: %s", resp.StatusCode, string(bodyBytes)),
			req.PAT)
	}

	// Parse response to extract web_url.
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", redactError(fmt.Errorf("parse response: %w", err), req.PAT)
	}

	webURL, ok := result["web_url"].(string)
	if !ok || webURL == "" {
		return "", redactError(fmt.Errorf("no web_url in response"), req.PAT)
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

// redactError replaces any occurrence of the PAT in error messages with [REDACTED].
// It handles both literal PAT and URL-encoded variants.
func redactError(err error, pat string) error {
	if err == nil {
		return nil
	}
	if pat == "" {
		return err
	}

	msg := err.Error()
	modified := false

	// Redact literal PAT.
	if strings.Contains(msg, pat) {
		msg = strings.ReplaceAll(msg, pat, "[REDACTED]")
		modified = true
	}

	// Redact URL-encoded PAT (e.g., in URLs or query strings).
	// Common URL encoding characters that might appear: space->%20, @->%40, etc.
	encodedPAT := strings.ReplaceAll(strings.ReplaceAll(pat, " ", "%20"), "@", "%40")
	if encodedPAT != pat && strings.Contains(msg, encodedPAT) {
		msg = strings.ReplaceAll(msg, encodedPAT, "[REDACTED]")
		modified = true
	}

	if modified {
		return fmt.Errorf("%s", msg)
	}
	return err
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

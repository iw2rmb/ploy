package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
// Retries on 429 (rate limit) and 5xx (server errors) with exponential backoff (max 3 attempts).
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

	// Retry logic with exponential backoff for 429 and 5xx errors (max 3 attempts).
	const maxRetries = 3
	const baseDelay = 1 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
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
			lastErr = err
			if attempt < maxRetries-1 && c.shouldRetry(ctx, err, 0) {
				c.backoff(ctx, attempt, baseDelay)
				continue
			}
			return "", redactError(fmt.Errorf("send request: %w", err), req.PAT)
		}

		// Read response body.
		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", redactError(fmt.Errorf("read response: %w", err), req.PAT)
		}

		// Check response status.
		if resp.StatusCode == http.StatusCreated {
			// Success: parse response to extract web_url.
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

		// Check if we should retry (429 or 5xx).
		lastErr = fmt.Errorf("gitlab api error: status %d: %s", resp.StatusCode, string(bodyBytes))
		if attempt < maxRetries-1 && c.shouldRetry(ctx, nil, resp.StatusCode) {
			c.backoff(ctx, attempt, baseDelay)
			continue
		}

		// Non-retryable error or final attempt.
		return "", redactError(lastErr, req.PAT)
	}

	// All retries exhausted.
	return "", redactError(fmt.Errorf("max retries exhausted: %w", lastErr), req.PAT)
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

// shouldRetry determines if a request should be retried based on the error or HTTP status code.
// Retries on 429 (Too Many Requests) and 5xx (server errors).
func (c *MRClient) shouldRetry(ctx context.Context, err error, statusCode int) bool {
	// Check context cancellation.
	if ctx.Err() != nil {
		return false
	}

	// Retry on network errors (except context cancellation).
	if err != nil {
		return true
	}

	// Retry on 429 (rate limit) or 5xx (server errors).
	return statusCode == http.StatusTooManyRequests || (statusCode >= 500 && statusCode < 600)
}

// backoff sleeps for an exponentially increasing duration before retrying.
// Base delay is 1 second; each retry doubles the delay (1s, 2s, 4s).
func (c *MRClient) backoff(ctx context.Context, attempt int, baseDelay time.Duration) {
	// Calculate delay: baseDelay * 2^attempt.
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))

	// Sleep with context awareness.
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	}
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

	// Build a set of variants to redact: literal, query-escaped, path-escaped,
	// and a minimal legacy replacement used in early code paths.
	variants := map[string]struct{}{
		pat: {},
	}
	if q := url.QueryEscape(pat); q != pat {
		variants[q] = struct{}{}
		// Some logs render spaces as %20 not "+"; include that form.
		variants[strings.ReplaceAll(q, "+", "%20")] = struct{}{}
	}
	if p := url.PathEscape(pat); p != pat {
		variants[p] = struct{}{}
	}
	// Legacy minimal encoding coverage.
	variants[strings.ReplaceAll(strings.ReplaceAll(pat, " ", "%20"), "@", "%40")] = struct{}{}

	modified := false
	for v := range variants {
		if v == "" || v == msg {
			continue
		}
		if strings.Contains(msg, v) {
			msg = strings.ReplaceAll(msg, v, "[REDACTED]")
			modified = true
		}
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

package arf

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/api/arf/models"
)

// OpenRewriteClient implements HTTP client for OpenRewrite service
type OpenRewriteClient struct {
	baseURL string
	client  *http.Client
}

// TransformRequest represents a transformation request to the service
type TransformRequest struct {
	JobID        string       `json:"job_id"`
	TarArchive   string       `json:"tar_archive"`   // base64 encoded
	RecipeConfig RecipeConfig `json:"recipe_config"`
}

// RecipeConfig represents OpenRewrite recipe configuration
type RecipeConfig struct {
	Recipe    string `json:"recipe"`
	Artifacts string `json:"artifacts,omitempty"`
}

// CreateJobRequest represents an async job creation request
type CreateJobRequest struct {
	TarArchive   string       `json:"tar_archive"`
	RecipeConfig RecipeConfig `json:"recipe_config"`
}

// JobResponse represents a job creation response
type JobResponse struct {
	JobID string `json:"job_id"`
}

// JobStatus represents job status information
type JobStatus struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`    // pending, running, completed, failed
	Progress  int    `json:"progress"`  // 0-100
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time,omitempty"`
	Error     string `json:"error,omitempty"`
}

// NewOpenRewriteClient creates a new HTTP client for OpenRewrite service
func NewOpenRewriteClient() *OpenRewriteClient {
	// Get platform domain
	platformDomain := os.Getenv("PLOY_PLATFORM_DOMAIN")
	if platformDomain == "" {
		platformDomain = "ployman.app"
	}
	
	baseURL := fmt.Sprintf("https://openrewrite.%s", platformDomain)
	
	// Allow override for development
	if override := os.Getenv("OPENREWRITE_SERVICE_URL"); override != "" {
		baseURL = override
	}
	
	return &OpenRewriteClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// Execute implements the TransformationEngine interface for HTTP service calls
func (c *OpenRewriteClient) Execute(ctx context.Context, step *models.RecipeStep, repoPath string) (*TransformationResult, error) {
	// Parse recipe configuration
	recipe, ok := step.Config["recipe"].(string)
	if !ok {
		return nil, fmt.Errorf("OpenRewrite step missing recipe configuration")
	}
	
	// Create tar archive of the repository (simplified - would need actual tar creation)
	// For now, return a placeholder implementation
	tarData := []byte("placeholder-tar-data") // TODO: Implement actual tar creation
	
	// Determine recipe artifacts
	artifacts := c.getRecipeArtifacts(recipe)
	
	// Call service transform endpoint
	result, err := c.Transform(tarData, RecipeConfig{
		Recipe:    recipe,
		Artifacts: artifacts,
	})
	if err != nil {
		return nil, fmt.Errorf("OpenRewrite service call failed: %w", err)
	}
	
	return result, nil
}

// Transform calls the synchronous transformation endpoint
func (c *OpenRewriteClient) Transform(tarData []byte, recipe RecipeConfig) (*TransformationResult, error) {
	req := TransformRequest{
		JobID:        uuid.New().String(),
		TarArchive:   base64.StdEncoding.EncodeToString(tarData),
		RecipeConfig: recipe,
	}
	
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	resp, err := c.client.Post(
		fmt.Sprintf("%s/v1/openrewrite/transform", c.baseURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("service returned status %d", resp.StatusCode)
	}
	
	var result TransformationResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return &result, nil
}

// CreateJob creates an asynchronous transformation job
func (c *OpenRewriteClient) CreateJob(tarData []byte, recipe RecipeConfig) (string, error) {
	req := CreateJobRequest{
		TarArchive:   base64.StdEncoding.EncodeToString(tarData),
		RecipeConfig: recipe,
	}
	
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}
	
	resp, err := c.client.Post(
		fmt.Sprintf("%s/v1/openrewrite/jobs", c.baseURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("service returned status %d", resp.StatusCode)
	}
	
	var result JobResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	
	return result.JobID, nil
}

// GetJobStatus retrieves the status of an asynchronous job
func (c *OpenRewriteClient) GetJobStatus(jobID string) (*JobStatus, error) {
	resp, err := c.client.Get(
		fmt.Sprintf("%s/v1/openrewrite/jobs/%s/status", c.baseURL, jobID),
	)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("service returned status %d", resp.StatusCode)
	}
	
	var status JobStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return &status, nil
}

// GetJobDiff retrieves the diff for a completed job
func (c *OpenRewriteClient) GetJobDiff(jobID string) ([]byte, error) {
	resp, err := c.client.Get(
		fmt.Sprintf("%s/v1/openrewrite/jobs/%s/diff", c.baseURL, jobID),
	)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("service returned status %d", resp.StatusCode)
	}
	
	// Read response body as raw bytes (could be text diff or binary)
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	return buf.Bytes(), nil
}

// Health checks if the OpenRewrite service is healthy
func (c *OpenRewriteClient) Health() error {
	resp, err := c.client.Get(fmt.Sprintf("%s/v1/openrewrite/health", c.baseURL))
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service unhealthy, status: %d", resp.StatusCode)
	}
	
	return nil
}

// getRecipeArtifacts returns the Maven coordinates for recipe artifacts
func (c *OpenRewriteClient) getRecipeArtifacts(recipe string) string {
	// Map common recipes to their artifacts (same logic as embedded engine)
	recipeMap := map[string]string{
		"org.openrewrite.java.migrate.Java11toJava17":            "org.openrewrite.recipe:rewrite-migrate-java:2.5.0",
		"org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0": "org.openrewrite.recipe:rewrite-spring:5.7.0",
		"org.openrewrite.java.spring.boot3.SpringBoot3BestPractices": "org.openrewrite.recipe:rewrite-spring:5.7.0",
		"org.openrewrite.java.cleanup.UnnecessaryThrows":         "org.openrewrite:rewrite-java:8.21.0",
	}
	
	if artifacts, ok := recipeMap[recipe]; ok {
		return artifacts
	}
	
	// Default to core Java recipes
	return "org.openrewrite:rewrite-java:8.21.0"
}

// ConfigureForJavaMigration configures client for Java migration (compatibility method)
func (c *OpenRewriteClient) ConfigureForJavaMigration() {
	// Client doesn't need local configuration - service handles this
	// This method exists for interface compatibility
}
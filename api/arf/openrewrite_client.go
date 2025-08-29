package arf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"
	
	"github.com/iw2rmb/ploy/api/arf/models"
	"github.com/iw2rmb/ploy/internal/storage"
)

// OpenRewriteClient handles OpenRewrite transformations via batch jobs
type OpenRewriteClient struct {
	dispatcher *OpenRewriteDispatcher
}

// NewOpenRewriteClient creates a new OpenRewrite client using the dispatcher
func NewOpenRewriteClient() *OpenRewriteClient {
	// Get Nomad and Consul addresses from environment
	nomadAddr := os.Getenv("NOMAD_ADDR")
	if nomadAddr == "" {
		nomadAddr = "http://localhost:4646"
	}
	
	consulAddr := os.Getenv("CONSUL_HTTP_ADDR")
	if consulAddr == "" {
		consulAddr = "http://localhost:8500"
	}
	
	// Create storage client (simplified for now)
	seaweedClient, err := storage.NewSeaweedFSClient(storage.SeaweedFSConfig{
		Master: "localhost:9333",
		Filer:  "localhost:8888",
		Collection: "ploy-artifacts",
		Replication: "001",
		Timeout: 30,
	})
	
	var storageClient *storage.StorageClient
	if err == nil && seaweedClient != nil {
		// Wrap SeaweedFS client in StorageClient with default config
		storageClient = storage.NewStorageClient(seaweedClient, nil)
	}
	
	// Create dispatcher
	dispatcher, err := NewOpenRewriteDispatcher(nomadAddr, consulAddr, storageClient)
	if err != nil {
		// Log error but continue with nil dispatcher
		fmt.Printf("Warning: failed to create dispatcher: %v\n", err)
	}
	
	return &OpenRewriteClient{
		dispatcher: dispatcher,
	}
}

// Execute implements the TransformationEngine interface for batch job execution
func (c *OpenRewriteClient) Execute(ctx context.Context, step *models.RecipeStep, repoPath string) (*TransformationResult, error) {
	// Parse recipe configuration
	recipe, ok := step.Config["recipe"].(string)
	if !ok {
		return nil, fmt.Errorf("OpenRewrite step missing recipe configuration")
	}
	
	// Create tar archive of the repository
	// TODO: Implement actual tar creation from repoPath
	tarData := []byte("placeholder-tar-data")
	
	// Transform recipe name to our simplified format
	simplifiedRecipe := c.simplifyRecipeName(recipe)
	
	// Submit and wait for completion
	return c.Transform(ctx, tarData, simplifiedRecipe)
}

// Transform submits a transformation job and waits for completion
func (c *OpenRewriteClient) Transform(ctx context.Context, tarArchive []byte, recipe string) (*TransformationResult, error) {
	if c.dispatcher == nil {
		return nil, fmt.Errorf("dispatcher not initialized")
	}
	
	// Submit job
	job, err := c.dispatcher.SubmitJob(ctx, recipe, bytes.NewReader(tarArchive))
	if err != nil {
		return nil, fmt.Errorf("failed to submit job: %w", err)
	}
	
	// Poll for completion (with timeout)
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("transformation timeout after 5 minutes")
		case <-ticker.C:
			// Check job status
			updatedJob, err := c.dispatcher.GetJob(ctx, job.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to get job status: %w", err)
			}
			
			switch updatedJob.Status {
			case "completed":
				// Success - build result
				result := &TransformationResult{
					RecipeID:       recipe,
					Success:        true,
					ExecutionTime:  updatedJob.CompletedAt.Sub(*updatedJob.StartedAt),
					ChangesApplied: 0,
					FilesModified:  []string{},
				}
				
				// Add result metadata if available
				if updatedJob.Result != nil {
					if changed, ok := updatedJob.Result["files_changed"].(float64); ok {
						result.ChangesApplied = int(changed)
					}
					if totalFiles, ok := updatedJob.Result["total_files"].(float64); ok {
						result.TotalFiles = int(totalFiles)
					}
				}
				
				return result, nil
				
			case "failed":
				// Failure
				return &TransformationResult{
					RecipeID: recipe,
					Success:  false,
					Errors: []TransformationError{
						{
							Type:    "job_failed",
							Message: updatedJob.Error,
						},
					},
				}, nil
			}
		}
	}
}

// CreateJob submits a job without waiting for completion
func (c *OpenRewriteClient) CreateJob(tarData []byte, recipe RecipeConfig) (string, error) {
	if c.dispatcher == nil {
		return "", fmt.Errorf("dispatcher not initialized")
	}
	
	simplifiedRecipe := c.simplifyRecipeName(recipe.Recipe)
	job, err := c.dispatcher.SubmitJob(context.Background(), simplifiedRecipe, bytes.NewReader(tarData))
	if err != nil {
		return "", fmt.Errorf("failed to submit job: %w", err)
	}
	
	return job.ID, nil
}

// GetJobStatus retrieves the status of a job
func (c *OpenRewriteClient) GetJobStatus(jobID string) (*JobStatus, error) {
	if c.dispatcher == nil {
		return nil, fmt.Errorf("dispatcher not initialized")
	}
	
	job, err := c.dispatcher.GetJob(context.Background(), jobID)
	if err != nil {
		return nil, err
	}
	
	status := &JobStatus{
		JobID:  job.ID,
		Status: job.Status,
	}
	
	if job.StartedAt != nil {
		status.StartTime = job.StartedAt.Format(time.RFC3339)
	}
	if job.CompletedAt != nil {
		status.EndTime = job.CompletedAt.Format(time.RFC3339)
	}
	if job.Error != "" {
		status.Error = job.Error
	}
	
	// Calculate progress based on status
	switch job.Status {
	case "pending":
		status.Progress = 0
	case "submitted", "running":
		status.Progress = 50
	case "completed":
		status.Progress = 100
	case "failed":
		status.Progress = 100
	}
	
	return status, nil
}

// GetJobDiff retrieves the diff for a completed job (not implemented for batch jobs)
func (c *OpenRewriteClient) GetJobDiff(jobID string) ([]byte, error) {
	// Diffs are embedded in the output tar file
	return nil, fmt.Errorf("diff retrieval not implemented for batch jobs")
}

// Health checks system health by verifying queue depth
func (c *OpenRewriteClient) Health() error {
	if c.dispatcher == nil {
		return fmt.Errorf("dispatcher not initialized")
	}
	
	depth, err := c.dispatcher.GetQueueDepth(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get queue depth: %w", err)
	}
	
	// Warn if queue is too deep
	if depth > 100 {
		return fmt.Errorf("queue depth too high: %d jobs pending", depth)
	}
	
	return nil
}

// ConfigureForJavaMigration is a compatibility method (no-op for batch jobs)
func (c *OpenRewriteClient) ConfigureForJavaMigration() {
	// Batch jobs handle configuration internally
}

// simplifyRecipeName converts full recipe names to simplified versions
func (c *OpenRewriteClient) simplifyRecipeName(recipe string) string {
	// Map full recipe names to simplified versions that native binary understands
	simplifiedMap := map[string]string{
		"org.openrewrite.java.migrate.Java11toJava17": "java11to17",
		"org.openrewrite.java.migrate.UpgradeToJava17": "java11to17",
		"org.openrewrite.java.migrate.Java8toJava11": "java8to11",
		"org.openrewrite.java.migrate.UpgradeToJava11": "java8to11",
	}
	
	if simplified, ok := simplifiedMap[recipe]; ok {
		return simplified
	}
	
	// Return as-is if not in map
	return recipe
}

// RecipeConfig for compatibility
type RecipeConfig struct {
	Recipe    string `json:"recipe"`
	Artifacts string `json:"artifacts,omitempty"`
}

// JobStatus for compatibility
type JobStatus struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time,omitempty"`
	Error     string `json:"error,omitempty"`
}
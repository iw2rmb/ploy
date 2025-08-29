package arf

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hashicorp/nomad/api"
	"github.com/iw2rmb/ploy/api/arf/storage"
	"log"
)

// OpenRewriteDispatcher handles dispatching OpenRewrite transformations to Nomad
type OpenRewriteDispatcher struct {
	nomadClient    *api.Client
	storageClient  storage.StorageService
	registryURL    string
	seaweedfsURL   string
	apiURL         string
}

// NewOpenRewriteDispatcher creates a new OpenRewrite dispatcher
func NewOpenRewriteDispatcher(nomadAddr, registryURL, seaweedfsURL, apiURL string, storageClient storage.StorageService) (*OpenRewriteDispatcher, error) {
	config := api.DefaultConfig()
	if nomadAddr != "" {
		config.Address = nomadAddr
	}
	
	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nomad client: %w", err)
	}
	
	return &OpenRewriteDispatcher{
		nomadClient:   client,
		storageClient: storageClient,
		registryURL:   registryURL,
		seaweedfsURL:  seaweedfsURL,
		apiURL:        apiURL,
	}, nil
}

// OpenRewriteRecipeRequest represents a request to execute an OpenRewrite recipe
type OpenRewriteRecipeRequest struct {
	RecipeClass    string `json:"recipe_class"`
	RecipeGroup    string `json:"recipe_group"`
	RecipeArtifact string `json:"recipe_artifact"`
	RecipeVersion  string `json:"recipe_version"`
	RepoPath       string `json:"repo_path"`
	JobID          string `json:"job_id"`
}

// ExecuteOpenRewriteRecipe dispatches an OpenRewrite transformation to Nomad
func (d *OpenRewriteDispatcher) ExecuteOpenRewriteRecipe(ctx context.Context, req *OpenRewriteRecipeRequest) (*TransformationResult, error) {
	log.Printf("Dispatching OpenRewrite recipe to Nomad: recipe=%s, coords=%s:%s:%s", 
		req.RecipeClass, req.RecipeGroup, req.RecipeArtifact, req.RecipeVersion)
	
	// Create a unique job ID if not provided
	if req.JobID == "" {
		req.JobID = fmt.Sprintf("openrewrite-%d", time.Now().Unix())
	}
	
	// Package the repository as tar
	inputTarPath := fmt.Sprintf("/tmp/%s-input.tar", req.JobID)
	if err := d.createTarFromRepo(req.RepoPath, inputTarPath); err != nil {
		return nil, fmt.Errorf("failed to create input tar: %w", err)
	}
	defer os.Remove(inputTarPath)
	
	// Upload input tar to storage
	inputStorageKey := fmt.Sprintf("openrewrite/%s/input.tar", req.JobID)
	if err := d.uploadToStorage(ctx, inputTarPath, inputStorageKey); err != nil {
		return nil, fmt.Errorf("failed to upload input tar: %w", err)
	}
	
	// Create Nomad job
	job := d.createNomadJob(req)
	
	// Submit job to Nomad
	_, _, err := d.nomadClient.Jobs().Register(job, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to submit Nomad job: %w", err)
	}
	
	// Wait for job completion
	result, err := d.waitForJobCompletion(ctx, req.JobID)
	if err != nil {
		return nil, fmt.Errorf("job execution failed: %w", err)
	}
	
	// Download and extract output
	outputStorageKey := fmt.Sprintf("openrewrite/%s/output.tar", req.JobID)
	outputTarPath := fmt.Sprintf("/tmp/%s-output.tar", req.JobID)
	defer os.Remove(outputTarPath)
	
	if err := d.downloadFromStorage(ctx, outputStorageKey, outputTarPath); err != nil {
		return nil, fmt.Errorf("failed to download output tar: %w", err)
	}
	
	// Extract output back to repo
	if err := d.extractTarToRepo(outputTarPath, req.RepoPath); err != nil {
		return nil, fmt.Errorf("failed to extract output tar: %w", err)
	}
	
	// Generate diff
	diff, err := d.generateDiff(req.RepoPath)
	if err != nil {
		log.Printf("Warning: Failed to generate diff: %v", err)
		diff = ""
	}
	
	// Build result
	result.Diff = diff
	result.RecipeID = req.RecipeClass
	
	return result, nil
}

// createNomadJob creates a Nomad job for OpenRewrite transformation
func (d *OpenRewriteDispatcher) createNomadJob(req *OpenRewriteRecipeRequest) *api.Job {
	jobID := req.JobID
	jobType := "batch"
	priority := 50
	datacenters := []string{"dc1"}
	
	// Create task group
	taskGroup := &api.TaskGroup{
		Name: stringPtr("openrewrite"),
		Tasks: []*api.Task{
			{
				Name:   "transform",
				Driver: "docker",
				Config: map[string]interface{}{
					"image": fmt.Sprintf("%s/openrewrite-jvm:latest", d.registryURL),
					"volumes": []string{
						"/tmp/openrewrite:/workspace",
					},
					"command": "/runner.sh",
					"args": []string{
						"/workspace/input.tar",
						"/workspace/output.tar",
						req.RecipeClass,
					},
				},
				Env: map[string]string{
					"RECIPE":          req.RecipeClass,
					"RECIPE_GROUP":    req.RecipeGroup,
					"RECIPE_ARTIFACT": req.RecipeArtifact,
					"RECIPE_VERSION":  req.RecipeVersion,
					"SEAWEEDFS_URL":   d.seaweedfsURL,
					"PLOY_API_URL":    d.apiURL,
					"MAVEN_CACHE_PATH": "maven-repository",
				},
				Resources: &api.Resources{
					CPU:      intPtr(500),
					MemoryMB: intPtr(2048),
				},
				// Add artifact download/upload tasks
				Artifacts: []*api.TaskArtifact{
					{
						GetterSource: stringPtr(fmt.Sprintf("%s/openrewrite/%s/input.tar", d.seaweedfsURL, req.JobID)),
						RelativeDest: stringPtr("/workspace/"),
					},
				},
			},
		},
	}
	
	// Create job
	job := &api.Job{
		ID:          &jobID,
		Name:        &jobID,
		Type:        &jobType,
		Priority:    &priority,
		Datacenters: datacenters,
		TaskGroups:  []*api.TaskGroup{taskGroup},
		Meta: map[string]string{
			"recipe":     req.RecipeClass,
			"repository": req.RepoPath,
		},
	}
	
	return job
}

// waitForJobCompletion waits for Nomad job to complete
func (d *OpenRewriteDispatcher) waitForJobCompletion(ctx context.Context, jobID string) (*TransformationResult, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	timeout := time.After(10 * time.Minute)
	
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("job execution timeout")
		case <-ticker.C:
			job, _, err := d.nomadClient.Jobs().Info(jobID, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get job info: %w", err)
			}
			
			if job.Status != nil && *job.Status == "dead" {
				// Check if job succeeded
				if job.StatusDescription != nil && strings.Contains(*job.StatusDescription, "completed") {
					return &TransformationResult{
						Success:        true,
						ExecutionTime:  10 * time.Second, // TODO: Calculate actual time
						ChangesApplied: 1,
					}, nil
				}
				return nil, fmt.Errorf("job failed: %s", *job.StatusDescription)
			}
		}
	}
}

// Helper functions for tar operations and storage
func (d *OpenRewriteDispatcher) createTarFromRepo(repoPath, tarPath string) error {
	// Use tar command to create archive
	cmd := fmt.Sprintf("tar -cf %s -C %s .", tarPath, repoPath)
	return executeCommand(cmd)
}

func (d *OpenRewriteDispatcher) extractTarToRepo(tarPath, repoPath string) error {
	// Use tar command to extract archive
	cmd := fmt.Sprintf("tar -xf %s -C %s", tarPath, repoPath)
	return executeCommand(cmd)
}

func (d *OpenRewriteDispatcher) uploadToStorage(ctx context.Context, filePath, storageKey string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	
	return d.storageClient.Put(ctx, storageKey, data)
}

func (d *OpenRewriteDispatcher) downloadFromStorage(ctx context.Context, storageKey, filePath string) error {
	data, err := d.storageClient.Get(ctx, storageKey)
	if err != nil {
		return err
	}
	
	return os.WriteFile(filePath, data, 0644)
}

func (d *OpenRewriteDispatcher) generateDiff(repoPath string) (string, error) {
	cmd := fmt.Sprintf("cd %s && git diff", repoPath)
	output, err := executeCommandWithOutput(cmd)
	if err != nil {
		return "", err
	}
	return output, nil
}

// Helper functions
func executeCommand(cmd string) error {
	return executeCommandWithError(cmd)
}

func executeCommandWithOutput(cmd string) (string, error) {
	// Use shell to execute command and capture output
	cmdParts := []string{"sh", "-c", cmd}
	output, err := exec.Command(cmdParts[0], cmdParts[1:]...).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func executeCommandWithError(cmd string) error {
	// Use shell to execute command
	cmdParts := []string{"sh", "-c", cmd}
	return exec.Command(cmdParts[0], cmdParts[1:]...).Run()
}

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

// ParseOpenRewriteRecipeID parses an OpenRewrite recipe ID into its components
func ParseOpenRewriteRecipeID(recipeID string) (*OpenRewriteRecipeRequest, error) {
	// OpenRewrite recipes typically follow the pattern:
	// org.openrewrite.java.migrate.Java8toJava11
	// We need to map this to Maven coordinates
	
	// Default values for common OpenRewrite recipes
	recipeMap := map[string]*OpenRewriteRecipeRequest{
		"org.openrewrite.java.migrate.Java8toJava11": {
			RecipeClass:    "org.openrewrite.java.migrate.Java8toJava11",
			RecipeGroup:    "org.openrewrite.recipe",
			RecipeArtifact: "rewrite-migrate-java",
			RecipeVersion:  "2.11.0",
		},
		"org.openrewrite.java.migrate.Java11toJava17": {
			RecipeClass:    "org.openrewrite.java.migrate.Java11toJava17",
			RecipeGroup:    "org.openrewrite.recipe",
			RecipeArtifact: "rewrite-migrate-java",
			RecipeVersion:  "2.11.0",
		},
		"org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0": {
			RecipeClass:    "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0",
			RecipeGroup:    "org.openrewrite.recipe",
			RecipeArtifact: "rewrite-spring",
			RecipeVersion:  "5.7.0",
		},
		"org.openrewrite.java.testing.junit5.JUnit5BestPractices": {
			RecipeClass:    "org.openrewrite.java.testing.junit5.JUnit5BestPractices",
			RecipeGroup:    "org.openrewrite.recipe",
			RecipeArtifact: "rewrite-testing-frameworks",
			RecipeVersion:  "2.11.0",
		},
	}
	
	if req, ok := recipeMap[recipeID]; ok {
		return req, nil
	}
	
	// For unknown recipes, try to infer from the ID
	// Default to rewrite-migrate-java for Java migration recipes
	if strings.Contains(recipeID, "migrate") {
		return &OpenRewriteRecipeRequest{
			RecipeClass:    recipeID,
			RecipeGroup:    "org.openrewrite.recipe",
			RecipeArtifact: "rewrite-migrate-java",
			RecipeVersion:  "2.11.0",
		}, nil
	}
	
	// Default fallback
	return &OpenRewriteRecipeRequest{
		RecipeClass:    recipeID,
		RecipeGroup:    "org.openrewrite.recipe",
		RecipeArtifact: "rewrite-migrate-java",
		RecipeVersion:  "2.11.0",
	}, nil
}
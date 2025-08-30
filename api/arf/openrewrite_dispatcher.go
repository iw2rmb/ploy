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
	log.Printf("[OpenRewriteDispatcher] Initializing with parameters:")
	log.Printf("  - Nomad Address: %s", nomadAddr)
	log.Printf("  - Registry URL: %s", registryURL) 
	log.Printf("  - SeaweedFS URL: %s", seaweedfsURL)
	log.Printf("  - API URL: %s", apiURL)
	log.Printf("  - Storage Client: %T", storageClient)
	
	if storageClient == nil {
		return nil, fmt.Errorf("storage client cannot be nil")
	}
	
	config := api.DefaultConfig()
	if nomadAddr != "" {
		config.Address = nomadAddr
	}
	log.Printf("[OpenRewriteDispatcher] Creating Nomad client with address: %s", config.Address)
	
	client, err := api.NewClient(config)
	if err != nil {
		log.Printf("[OpenRewriteDispatcher] ERROR: Failed to create Nomad client: %v", err)
		return nil, fmt.Errorf("failed to create Nomad client: %w", err)
	}
	
	log.Printf("[OpenRewriteDispatcher] Nomad client created successfully")
	
	dispatcher := &OpenRewriteDispatcher{
		nomadClient:   client,
		storageClient: storageClient,
		registryURL:   registryURL,
		seaweedfsURL:  seaweedfsURL,
		apiURL:        apiURL,
	}
	
	log.Printf("[OpenRewriteDispatcher] Dispatcher created successfully")
	return dispatcher, nil
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
	log.Printf("[OpenRewrite Dispatcher] ===== ENTRY POINT =====")
	log.Printf("[OpenRewrite Dispatcher] Function called with context deadline: %v", ctx.Done())
	log.Printf("[OpenRewrite Dispatcher] Dispatcher instance: %p", d)
	
	if d == nil {
		log.Printf("[OpenRewrite Dispatcher] CRITICAL ERROR: Dispatcher is nil!")
		return nil, fmt.Errorf("dispatcher is nil")
	}
	
	if req == nil {
		log.Printf("[OpenRewrite Dispatcher] CRITICAL ERROR: Request is nil!")
		return nil, fmt.Errorf("request cannot be nil")
	}
	
	log.Printf("[OpenRewrite Dispatcher] Starting dispatch for recipe=%s, repo=%s", 
		req.RecipeClass, req.RepoPath)
	log.Printf("[OpenRewrite Dispatcher] Nomad client: %p, Storage client: %p", d.nomadClient, d.storageClient)
	
	// Create a unique job ID if not provided
	if req.JobID == "" {
		req.JobID = fmt.Sprintf("openrewrite-%d", time.Now().Unix())
	}
	log.Printf("[OpenRewrite Dispatcher] Job ID: %s", req.JobID)
	
	// Package the repository as tar
	inputTarPath := fmt.Sprintf("/tmp/%s-input.tar", req.JobID)
	log.Printf("[OpenRewrite Dispatcher] Creating tar from repo: %s -> %s", req.RepoPath, inputTarPath)
	if err := d.createTarFromRepo(req.RepoPath, inputTarPath); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Failed to create tar: %v", err)
		return nil, fmt.Errorf("failed to create input tar: %w", err)
	}
	defer os.Remove(inputTarPath)
	
	// Check tar file size
	if fileInfo, err := os.Stat(inputTarPath); err == nil {
		log.Printf("[OpenRewrite Dispatcher] Tar file created: size=%d bytes", fileInfo.Size())
	}
	
	// Test storage connectivity first
	testKey := fmt.Sprintf("openrewrite/connectivity-test-%d", time.Now().Unix())
	testData := []byte("connectivity-test")
	log.Printf("[OpenRewrite Dispatcher] Testing storage connectivity with key: %s", testKey)
	if err := d.storageClient.Put(ctx, testKey, testData); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Storage connectivity test failed: %v", err)
		return nil, fmt.Errorf("storage not accessible: %w", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Storage connectivity test successful")
	
	// Clean up test file
	d.storageClient.Delete(ctx, testKey)
	
	// Upload input tar to storage
	inputStorageKey := fmt.Sprintf("openrewrite/%s/input.tar", req.JobID)
	
	// Get file size for logging
	fileInfo, _ := os.Stat(inputTarPath)
	fileSize := int64(0)
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}
	
	log.Printf("[OpenRewrite Dispatcher] Uploading tar to storage: key=%s, size=%d bytes", inputStorageKey, fileSize)
	if err := d.uploadToStorage(ctx, inputTarPath, inputStorageKey); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Failed to upload tar to key=%s: %v", inputStorageKey, err)
		return nil, fmt.Errorf("failed to upload input tar to %s: %w", inputStorageKey, err)
	}
	
	// Verify upload by checking if file exists
	log.Printf("[OpenRewrite Dispatcher] Verifying upload: checking if file exists at key=%s", inputStorageKey)
	exists, err := d.storageClient.Exists(ctx, inputStorageKey)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] WARNING: Failed to verify upload existence: %v", err)
	} else if !exists {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Upload verification failed - file does not exist at key=%s", inputStorageKey)
		return nil, fmt.Errorf("upload verification failed: file not found at %s", inputStorageKey)
	} else {
		log.Printf("[OpenRewrite Dispatcher] Upload verification successful - file exists at key=%s", inputStorageKey)
	}
	
	log.Printf("[OpenRewrite Dispatcher] Tar uploaded and verified successfully")
	
	// Create Nomad job
	log.Printf("[OpenRewrite Dispatcher] Creating Nomad job configuration")
	job := d.createNomadJob(req)
	
	// Log the download URL that will be used
	downloadURL := fmt.Sprintf("%s/openrewrite/%s/input.tar", d.seaweedfsURL, req.JobID)
	log.Printf("[OpenRewrite Dispatcher] Job config: ID=%s, Image=%s/openrewrite-jvm:latest", 
		*job.ID, d.registryURL)
	log.Printf("[OpenRewrite Dispatcher] Artifact download URL: %s", downloadURL)
	
	// Submit job to Nomad
	log.Printf("[OpenRewrite Dispatcher] Submitting job to Nomad at %s", d.nomadClient.Address())
	jobResp, _, err := d.nomadClient.Jobs().Register(job, nil)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Failed to submit job: %v", err)
		return nil, fmt.Errorf("failed to submit Nomad job: %w", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Job submitted successfully: EvalID=%s", jobResp.EvalID)
	
	// Wait for job completion
	log.Printf("[OpenRewrite Dispatcher] Waiting for job completion: %s", req.JobID)
	result, err := d.waitForJobCompletion(ctx, req.JobID)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Job execution failed: %v", err)
		return nil, fmt.Errorf("job execution failed: %w", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Job completed successfully")
	
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
					// Use custom OpenRewrite image from registry
					"image": fmt.Sprintf("%s/openrewrite-jvm:latest", d.registryURL),
					"volumes": []string{
						"/tmp/openrewrite:/workspace",
					},
					// Use custom image's default entrypoint (no command override needed)
					"dns_servers": []string{"172.17.0.1"},
					"dns_search_domains": []string{"service.consul"},
					"force_pull": false, // Don't force pull if image exists
				},
				Env: map[string]string{
					"RECIPE":           req.RecipeClass,
					"RECIPE_GROUP":     req.RecipeGroup,    // Empty for dynamic discovery
					"RECIPE_ARTIFACT":  req.RecipeArtifact, // Empty for dynamic discovery
					"RECIPE_VERSION":   req.RecipeVersion,  // Empty for dynamic discovery
					"SEAWEEDFS_URL":    "http://seaweedfs-filer.service.consul:8888",
					"PLOY_API_URL":     d.apiURL,
					"MAVEN_CACHE_PATH": "maven-repository",
					"DISCOVER_RECIPE":  "true", // Tell runner.sh to discover recipe coordinates
				},
				Resources: &api.Resources{
					CPU:      intPtr(500),
					MemoryMB: intPtr(2048),
				},
				// Add artifact download/upload tasks  
				Artifacts: []*api.TaskArtifact{
					{
						// Include bucket/collection prefix to match upload path
						GetterSource: stringPtr(fmt.Sprintf("%s/ploy-artifacts/openrewrite/%s/input.tar", d.seaweedfsURL, req.JobID)),
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
	startTime := time.Now()
	lastStatus := ""
	
	for {
		select {
		case <-ctx.Done():
			log.Printf("[OpenRewrite Dispatcher] Context cancelled while waiting for job %s", jobID)
			return nil, ctx.Err()
		case <-timeout:
			log.Printf("[OpenRewrite Dispatcher] Job %s timed out after 10 minutes", jobID)
			return nil, fmt.Errorf("job execution timeout")
		case <-ticker.C:
			job, _, err := d.nomadClient.Jobs().Info(jobID, nil)
			if err != nil {
				log.Printf("[OpenRewrite Dispatcher] ERROR: Failed to get job info for %s: %v", jobID, err)
				// Don't fail immediately, could be transient
				continue
			}
			
			// Log status changes
			currentStatus := ""
			if job.Status != nil {
				currentStatus = *job.Status
				if currentStatus != lastStatus {
					log.Printf("[OpenRewrite Dispatcher] Job %s status: %s (elapsed: %v)", 
						jobID, currentStatus, time.Since(startTime))
					lastStatus = currentStatus
					
					// Also check allocations for more details
					allocs, _, err := d.nomadClient.Jobs().Allocations(jobID, false, nil)
					if err == nil && len(allocs) > 0 {
						alloc := allocs[0]
						log.Printf("[OpenRewrite Dispatcher] Allocation %s: Status=%s, DesiredStatus=%s", 
							alloc.ID, alloc.ClientStatus, alloc.DesiredStatus)
						
						// Check task states
						if alloc.TaskStates != nil {
							for taskName, taskState := range alloc.TaskStates {
								log.Printf("[OpenRewrite Dispatcher] Task %s: State=%s, Failed=%v", 
									taskName, taskState.State, taskState.Failed)
								// Log any events
								for _, event := range taskState.Events {
									if event.DisplayMessage != "" {
										log.Printf("[OpenRewrite Dispatcher] Task event: %s", event.DisplayMessage)
									}
								}
							}
						}
					}
				}
			}
			
			if job.Status != nil && *job.Status == "dead" {
				// Check if job succeeded
				if job.StatusDescription != nil && strings.Contains(*job.StatusDescription, "completed") {
					log.Printf("[OpenRewrite Dispatcher] Job %s completed successfully", jobID)
					return &TransformationResult{
						Success:        true,
						ExecutionTime:  time.Since(startTime),
						ChangesApplied: 1,
					}, nil
				}
				log.Printf("[OpenRewrite Dispatcher] Job %s failed: %s", jobID, *job.StatusDescription)
				return nil, fmt.Errorf("job failed: %s", *job.StatusDescription)
			}
		}
	}
}

// Helper functions for tar operations and storage
func (d *OpenRewriteDispatcher) createTarFromRepo(repoPath, tarPath string) error {
	// Validate repo path exists
	if _, err := os.Stat(repoPath); err != nil {
		return fmt.Errorf("repository path does not exist: %s", repoPath)
	}
	
	// Remove existing tar file if it exists
	os.Remove(tarPath)
	
	// Use tar command to create archive
	cmd := fmt.Sprintf("tar -cf %s -C %s .", tarPath, repoPath)
	if err := executeCommand(cmd); err != nil {
		return fmt.Errorf("failed to create tar archive: %w", err)
	}
	
	// Verify tar file was created
	fileInfo, err := os.Stat(tarPath)
	if err != nil {
		return fmt.Errorf("tar file was not created: %w", err)
	}
	
	log.Printf("Created tar archive %s (size: %d bytes)", tarPath, fileInfo.Size())
	return nil
}

func (d *OpenRewriteDispatcher) extractTarToRepo(tarPath, repoPath string) error {
	// Use tar command to extract archive
	cmd := fmt.Sprintf("tar -xf %s -C %s", tarPath, repoPath)
	return executeCommand(cmd)
}

func (d *OpenRewriteDispatcher) uploadToStorage(ctx context.Context, filePath, storageKey string) error {
	// Check if file exists and get its size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}
	
	// Open file for reading
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()
	
	// Read entire file into memory (we know the size)
	data := make([]byte, fileInfo.Size())
	n, err := io.ReadFull(file, data)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	if int64(n) != fileInfo.Size() {
		return fmt.Errorf("read %d bytes but expected %d from %s", n, fileInfo.Size(), filePath)
	}
	
	// Upload to storage with retry logic
	var lastErr error
	for i := 0; i < 3; i++ {
		if err := d.storageClient.Put(ctx, storageKey, data); err != nil {
			lastErr = err
			log.Printf("Upload attempt %d failed for %s: %v", i+1, storageKey, err)
			time.Sleep(time.Second * time.Duration(i+1)) // Exponential backoff
			continue
		}
		log.Printf("Successfully uploaded %s to storage (size: %d bytes)", storageKey, len(data))
		return nil
	}
	
	return fmt.Errorf("failed to upload to storage after 3 attempts: %w", lastErr)
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
	// Pass the recipe class name directly to the OpenRewrite engine
	// The universal OpenRewrite image will discover the correct Maven coordinates dynamically
	// This allows any recipe to be used without hardcoding
	
	return &OpenRewriteRecipeRequest{
		RecipeClass:    recipeID,
		// These will be discovered by OpenRewrite CLI automatically
		RecipeGroup:    "",
		RecipeArtifact: "",
		RecipeVersion:  "",
	}, nil
}
package arf

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"log"

	"github.com/hashicorp/nomad/api"
	"github.com/iw2rmb/ploy/api/arf/storage"
)

// OpenRewriteDispatcher handles dispatching OpenRewrite transformations to Nomad
type OpenRewriteDispatcher struct {
	nomadClient   *api.Client
	storageClient storage.StorageService
	registryURL   string
	seaweedfsURL  string
	apiURL        string
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

	// Early infrastructure validation with short timeout
	log.Printf("[OpenRewrite Dispatcher] Validating Nomad infrastructure...")
	infraCtx, infraCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer infraCancel()
	
	// Test Nomad connectivity
	if _, err := d.nomadClient.Agent().Self(); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Nomad is not accessible: %v", err)
		return nil, fmt.Errorf("OpenRewrite infrastructure not ready - Nomad unreachable: %w", err)
	}
	
	// Check if we can list jobs (basic health check)  
	jobsDone := make(chan error, 1)
	go func() {
		_, _, err := d.nomadClient.Jobs().List(&api.QueryOptions{
			Region: "global",
			AllowStale: true,
		})
		jobsDone <- err
	}()
	
	select {
	case err := <-jobsDone:
		if err != nil {
			log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot query Nomad jobs: %v", err)
			return nil, fmt.Errorf("OpenRewrite infrastructure not ready - cannot query jobs: %w", err)
		}
	case <-infraCtx.Done():
		log.Printf("[OpenRewrite Dispatcher] ERROR: Nomad jobs query timed out")
		return nil, fmt.Errorf("OpenRewrite infrastructure not ready - Nomad query timed out")
	}
	
	log.Printf("[OpenRewrite Dispatcher] Infrastructure validation passed")
	
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

	// Upload input tar to storage (match artifacts path used by Nomad job)
	inputStorageKey := fmt.Sprintf("artifacts/openrewrite/%s/input.tar", req.JobID)

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
	downloadURL := fmt.Sprintf("%s/artifacts/openrewrite/%s/input.tar", d.seaweedfsURL, req.JobID)
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
	outputStorageKey := fmt.Sprintf("artifacts/openrewrite/%s/output.tar", req.JobID)
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
				Name:   "openrewrite",  // Changed from "transform" to match task group name
				Driver: "docker",
				Config: map[string]interface{}{
					// Use custom OpenRewrite image from registry (now with setup script)
					"image":              fmt.Sprintf("%s/openrewrite-jvm:latest", d.registryURL),
					"dns_servers":        []string{"172.17.0.1"},
					"dns_search_domains": []string{"service.consul"},
					"force_pull":         true, // Force pull to get latest image with setup script
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
					"ARTIFACT_URL":     fmt.Sprintf("%s/artifacts/openrewrite/%s/input.tar", d.seaweedfsURL, req.JobID), // Pass full artifact URL
					"OUTPUT_KEY":       fmt.Sprintf("artifacts/openrewrite/%s/output.tar", req.JobID), // Output storage key
				},
				Resources: &api.Resources{
					CPU:      intPtr(500),
					MemoryMB: intPtr(2048),
				},
				// Add artifact download/upload tasks
				Artifacts: []*api.TaskArtifact{
					{
						// Download artifact from SeaweedFS
						GetterSource: stringPtr(fmt.Sprintf("%s/artifacts/openrewrite/%s/input.tar", d.seaweedfsURL, req.JobID)),
						RelativeDest: stringPtr("local/"),  // Nomad extracts to local/ directory
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

	// 4-minute timeout allows proper job completion while staying under handler timeout
	timeout := time.After(4 * time.Minute)
	startTime := time.Now()
	lastStatus := ""

	for {
		select {
		case <-ctx.Done():
			log.Printf("[OpenRewrite Dispatcher] Context cancelled while waiting for job %s - performing cleanup", jobID)
			
			// Attempt to stop the job to free resources
			go func() {
				_, _, stopErr := d.nomadClient.Jobs().Deregister(jobID, false, nil)
				if stopErr != nil {
					log.Printf("[OpenRewrite Dispatcher] Warning: Failed to stop job %s after context cancellation: %v", jobID, stopErr)
				} else {
					log.Printf("[OpenRewrite Dispatcher] Job %s stopped successfully after context cancellation", jobID)
				}
			}()
			
			return nil, ctx.Err()
		case <-timeout:
			log.Printf("[OpenRewrite Dispatcher] Job %s timed out after 4 minutes", jobID)
			return nil, fmt.Errorf("job execution timeout after 4 minutes")
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
				// Enhanced failure detection - check allocations for actual task status
				allocs, _, allocErr := d.nomadClient.Jobs().Allocations(jobID, false, nil)
				if allocErr == nil && len(allocs) > 0 {
					alloc := allocs[0]
					log.Printf("[OpenRewrite Dispatcher] Analyzing allocation %s: Status=%s", alloc.ID, alloc.ClientStatus)
					
					// Check for explicit task failures
					if alloc.TaskStates != nil {
						for taskName, taskState := range alloc.TaskStates {
							if taskState.Failed {
								log.Printf("[OpenRewrite Dispatcher] Task %s failed: State=%s", taskName, taskState.State)
								// Check for specific failure reasons in events
								var failureReason string
								for _, event := range taskState.Events {
									if event.Type == "Driver Failure" || event.Type == "Task Setup" {
										failureReason = event.DisplayMessage
										break
									}
								}
								if failureReason == "" {
									failureReason = "Task execution failed"
								}
								return nil, fmt.Errorf("job failed: %s", failureReason)
							}
							
							// Check for successful completion
							if taskState.State == "dead" && !taskState.Failed {
								log.Printf("[OpenRewrite Dispatcher] Job %s completed successfully", jobID)
								return &TransformationResult{
									Success:        true,
									ExecutionTime:  time.Since(startTime),
									ChangesApplied: 1,
								}, nil
							}
						}
					}
					
					// Fall back to allocation status for failure detection
					if alloc.ClientStatus == "failed" {
						log.Printf("[OpenRewrite Dispatcher] Job %s failed: Allocation failed", jobID)
						return nil, fmt.Errorf("job failed: allocation status failed")
					}
				}
				
				// Original logic as fallback
				if job.StatusDescription != nil && strings.Contains(*job.StatusDescription, "completed") {
					log.Printf("[OpenRewrite Dispatcher] Job %s completed successfully", jobID)
					return &TransformationResult{
						Success:        true,
						ExecutionTime:  time.Since(startTime),
						ChangesApplied: 1,
					}, nil
				}
				
				statusDesc := "unknown failure"
				if job.StatusDescription != nil {
					statusDesc = *job.StatusDescription
				}
				log.Printf("[OpenRewrite Dispatcher] Job %s failed: %s", jobID, statusDesc)
				return nil, fmt.Errorf("job failed: %s", statusDesc)
			}
		}
	}
}

// Helper functions for tar operations and storage
func (d *OpenRewriteDispatcher) createTarFromRepo(repoPath, tarPath string) error {
	log.Printf("[OpenRewrite Dispatcher] ===== TAR CREATION START =====")
	log.Printf("[OpenRewrite Dispatcher] Source repo path: %s", repoPath)
	log.Printf("[OpenRewrite Dispatcher] Target tar path: %s", tarPath)
	
	// Validate repo path exists and analyze contents
	repoInfo, err := os.Stat(repoPath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Repository path does not exist: %s - %v", repoPath, err)
		return fmt.Errorf("repository path does not exist: %s", repoPath)
	}
	log.Printf("[OpenRewrite Dispatcher] Repository path exists: isDir=%v, size=%d", repoInfo.IsDir(), repoInfo.Size())
	
	// Count files in repository before tar creation
	fileCount := 0
	var totalSize int64
	err = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("[OpenRewrite Dispatcher] Warning: Error walking path %s: %v", path, err)
			return nil // Skip errors
		}
		if !info.IsDir() {
			fileCount++
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] Warning: Error analyzing repository: %v", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Repository analysis: %d files, %d bytes total", fileCount, totalSize)
	
	// List some sample files for debugging
	log.Printf("[OpenRewrite Dispatcher] Sample files in repository:")
	files, err := os.ReadDir(repoPath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] Warning: Cannot read directory: %v", err)
	} else {
		sampleCount := 0
		for _, file := range files {
			if sampleCount >= 10 {
				log.Printf("[OpenRewrite Dispatcher] ... and %d more files", len(files)-10)
				break
			}
			log.Printf("[OpenRewrite Dispatcher]   - %s (isDir: %v)", file.Name(), file.IsDir())
			sampleCount++
		}
	}

	// Remove existing tar file if it exists
	if _, err := os.Stat(tarPath); err == nil {
		log.Printf("[OpenRewrite Dispatcher] Removing existing tar file: %s", tarPath)
		if err := os.Remove(tarPath); err != nil {
			log.Printf("[OpenRewrite Dispatcher] Warning: Failed to remove existing tar file: %v", err)
		}
	}

	// Use tar command to create archive with comprehensive logging
	cmd := fmt.Sprintf("tar -cf %s -C %s .", tarPath, repoPath)
	log.Printf("[OpenRewrite Dispatcher] Executing tar command: %s", cmd)
	
	startTime := time.Now()
	if err := executeCommand(cmd); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Tar command failed after %v: %v", time.Since(startTime), err)
		log.Printf("[OpenRewrite Dispatcher] Command was: %s", cmd)
		return fmt.Errorf("failed to create tar archive: %w", err)
	}
	duration := time.Since(startTime)
	log.Printf("[OpenRewrite Dispatcher] Tar command completed successfully in %v", duration)

	// Verify tar file was created and analyze it
	fileInfo, err := os.Stat(tarPath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Tar file was not created: %s - %v", tarPath, err)
		return fmt.Errorf("tar file was not created: %w", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Tar file created successfully: %s", tarPath)
	log.Printf("[OpenRewrite Dispatcher] Tar file size: %d bytes", fileInfo.Size())

	// Test tar file integrity by listing contents
	log.Printf("[OpenRewrite Dispatcher] Verifying tar file integrity...")
	listCmd := fmt.Sprintf("tar -tf %s", tarPath)
	output, err := executeCommandWithOutput(listCmd)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] Warning: Cannot verify tar contents: %v", err)
	} else {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		log.Printf("[OpenRewrite Dispatcher] Tar contains %d entries", len(lines))
		// Show first few entries
		for i, line := range lines {
			if i >= 5 {
				log.Printf("[OpenRewrite Dispatcher] ... and %d more entries", len(lines)-5)
				break
			}
			log.Printf("[OpenRewrite Dispatcher]   - %s", line)
		}
	}

	log.Printf("[OpenRewrite Dispatcher] ===== TAR CREATION SUCCESS =====")
	log.Printf("Created tar archive %s (size: %d bytes)", tarPath, fileInfo.Size())
	return nil
}

func (d *OpenRewriteDispatcher) extractTarToRepo(tarPath, repoPath string) error {
	// Use tar command to extract archive
	cmd := fmt.Sprintf("tar -xf %s -C %s", tarPath, repoPath)
	return executeCommand(cmd)
}

func (d *OpenRewriteDispatcher) uploadToStorage(ctx context.Context, filePath, storageKey string) error {
	log.Printf("[OpenRewrite Dispatcher] ===== STORAGE UPLOAD START =====")
	log.Printf("[OpenRewrite Dispatcher] Local file path: %s", filePath)
	log.Printf("[OpenRewrite Dispatcher] Storage key: %s", storageKey)
	log.Printf("[OpenRewrite Dispatcher] SeaweedFS URL: %s", d.seaweedfsURL)
	
	// Check if file exists and get its size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot stat file %s: %v", filePath, err)
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}
	log.Printf("[OpenRewrite Dispatcher] File exists: size=%d bytes, mode=%v", fileInfo.Size(), fileInfo.Mode())

	// Verify file is readable and not empty
	if fileInfo.Size() == 0 {
		log.Printf("[OpenRewrite Dispatcher] ERROR: File is empty: %s", filePath)
		return fmt.Errorf("file is empty: %s", filePath)
	}

	// Open file for reading
	log.Printf("[OpenRewrite Dispatcher] Opening file for reading...")
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot open file %s: %v", filePath, err)
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()
	log.Printf("[OpenRewrite Dispatcher] File opened successfully")

	// Read entire file into memory (we know the size)
	log.Printf("[OpenRewrite Dispatcher] Reading file into memory (%d bytes)...", fileInfo.Size())
	data := make([]byte, fileInfo.Size())
	startRead := time.Now()
	n, err := io.ReadFull(file, data)
	readDuration := time.Since(startRead)
	if err != nil && err != io.EOF {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Failed to read file %s after %v: %v", filePath, readDuration, err)
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	if int64(n) != fileInfo.Size() {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Size mismatch - read %d bytes but expected %d from %s", n, fileInfo.Size(), filePath)
		return fmt.Errorf("read %d bytes but expected %d from %s", n, fileInfo.Size(), filePath)
	}
	log.Printf("[OpenRewrite Dispatcher] File read successfully: %d bytes in %v", n, readDuration)

	// Log storage client details
	log.Printf("[OpenRewrite Dispatcher] Storage client type: %T", d.storageClient)
	
	// Upload to storage with retry logic and detailed error tracking
	log.Printf("[OpenRewrite Dispatcher] Starting upload with retry logic...")
	var lastErr error
	for i := 0; i < 3; i++ {
		log.Printf("[OpenRewrite Dispatcher] Upload attempt %d/3 for key: %s", i+1, storageKey)
		attemptStart := time.Now()
		
		if err := d.storageClient.Put(ctx, storageKey, data); err != nil {
			attemptDuration := time.Since(attemptStart)
			lastErr = err
			log.Printf("[OpenRewrite Dispatcher] Upload attempt %d FAILED after %v: %v", i+1, attemptDuration, err)
			log.Printf("[OpenRewrite Dispatcher] Error type: %T", err)
			
			if i < 2 { // Don't sleep on the last attempt
				sleepDuration := time.Second * time.Duration(i+1)
				log.Printf("[OpenRewrite Dispatcher] Waiting %v before retry...", sleepDuration)
				time.Sleep(sleepDuration) // Exponential backoff
			}
			continue
		}
		
		attemptDuration := time.Since(attemptStart)
		log.Printf("[OpenRewrite Dispatcher] Upload attempt %d SUCCESS in %v", i+1, attemptDuration)
		break
	}
	
	if lastErr != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: All upload attempts failed. Final error: %v", lastErr)
		return fmt.Errorf("failed to upload to storage after 3 attempts: %w", lastErr)
	}

	log.Printf("[OpenRewrite Dispatcher] Upload completed successfully")

	// Verify upload by checking if file exists in storage
	log.Printf("[OpenRewrite Dispatcher] Verifying upload by checking storage existence...")
	exists, err := d.storageClient.Exists(ctx, storageKey)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot verify upload existence: %v", err)
		return fmt.Errorf("failed to verify upload existence: %w", err)
	}
	if !exists {
		log.Printf("[OpenRewrite Dispatcher] ERROR: File does not exist in storage after upload: %s", storageKey)
		return fmt.Errorf("file does not exist in storage after upload: %s", storageKey)
	}
	log.Printf("[OpenRewrite Dispatcher] Storage existence verified successfully")

	// Additional verification: construct HTTP URL and test accessibility
	httpURL := fmt.Sprintf("%s/%s", d.seaweedfsURL, storageKey)
	log.Printf("[OpenRewrite Dispatcher] HTTP URL for verification: %s", httpURL)
	
	// Test HTTP access using a simple HEAD request
	log.Printf("[OpenRewrite Dispatcher] Testing HTTP accessibility...")
	testCmd := fmt.Sprintf("curl -s -I --max-time 10 '%s'", httpURL)
	output, err := executeCommandWithOutput(testCmd)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] WARNING: HTTP accessibility test failed: %v", err)
		log.Printf("[OpenRewrite Dispatcher] Command was: %s", testCmd)
		// Don't fail the upload for this, it might be a network issue
	} else {
		log.Printf("[OpenRewrite Dispatcher] HTTP accessibility test result:")
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			log.Printf("[OpenRewrite Dispatcher]   %s", line)
		}
		
		// Check for successful HTTP status
		if strings.Contains(output, "200 OK") {
			log.Printf("[OpenRewrite Dispatcher] HTTP accessibility confirmed: 200 OK")
		} else if strings.Contains(output, "HTTP/") {
			log.Printf("[OpenRewrite Dispatcher] HTTP response received (may not be 200 OK)")
		} else {
			log.Printf("[OpenRewrite Dispatcher] WARNING: Unexpected HTTP response format")
		}
	}

	log.Printf("[OpenRewrite Dispatcher] ===== STORAGE UPLOAD SUCCESS =====")
	log.Printf("Successfully uploaded %s to storage (size: %d bytes)", storageKey, len(data))
	return nil
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
// This function now relies purely on type-based detection and dynamic discovery
// No shortcut recipe mappings are used - all recipes use full OpenRewrite class names
func ParseOpenRewriteRecipeID(recipeID string) (*OpenRewriteRecipeRequest, error) {
	// Validate that the recipe ID looks like a valid OpenRewrite recipe class
	if recipeID == "" {
		return nil, fmt.Errorf("recipe ID cannot be empty")
	}

	// All OpenRewrite recipes should use full class names (e.g., org.openrewrite.java.migrate.UpgradeToJava17)
	// Dynamic discovery will handle Maven coordinate resolution automatically
	return &OpenRewriteRecipeRequest{
		RecipeClass: recipeID,
		// OpenRewrite CLI will discover these automatically
		RecipeGroup:    "",
		RecipeArtifact: "",
		RecipeVersion:  "",
	}, nil
}

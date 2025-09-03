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
)

// OpenRewriteDispatcher handles dispatching OpenRewrite transformations to Nomad
type OpenRewriteDispatcher struct {
	nomadClient   *api.Client
	storageClient StorageService
	registryURL   string
	seaweedfsURL  string
	apiURL        string
}

// NewOpenRewriteDispatcher creates a new OpenRewrite dispatcher
func NewOpenRewriteDispatcher(nomadAddr, registryURL, seaweedfsURL, apiURL string, storageClient StorageService) (*OpenRewriteDispatcher, error) {
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
	RecipeClass      string `json:"recipe_class"`
	RecipeGroup      string `json:"recipe_group"`
	RecipeArtifact   string `json:"recipe_artifact"`
	RecipeVersion    string `json:"recipe_version"`
	RepoPath         string `json:"repo_path"`
	JobID            string `json:"job_id"`            // Nomad job ID (will be set after job submission)
	TransformationID string `json:"transformation_id"` // UUID from ARF handler
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
			Region:     "global",
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

	// Log the transformation ID from the handler
	log.Printf("[OpenRewrite Dispatcher] Transformation ID: %s", req.TransformationID)

	// Generate a Nomad job name using timestamp (Nomad will assign its own ID)
	nomadJobName := fmt.Sprintf("openrewrite-%d", time.Now().Unix())
	log.Printf("[OpenRewrite Dispatcher] Nomad job name: %s", nomadJobName)

	// Package the repository as tar
	inputTarPath := fmt.Sprintf("/tmp/%s-input.tar", req.TransformationID)
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

	// For now, we'll use the nomadJobName as the storage key since Nomad job names are unique
	// The actual Nomad job ID will be the same as the name we provide
	req.JobID = nomadJobName

	// Upload input tar to storage using the job name (which will be the job ID)
	inputStorageKey := fmt.Sprintf("jobs/%s/input.tar", req.JobID)

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

	// Create and submit Nomad job
	log.Printf("[OpenRewrite Dispatcher] Creating Nomad job configuration")
	
	// Log environment configuration for debugging
	log.Printf("[OpenRewrite Dispatcher] Container environment:")
	log.Printf("[OpenRewrite Dispatcher]   JOB_ID: %s", req.JobID)
	log.Printf("[OpenRewrite Dispatcher]   SEAWEEDFS_URL: %s", "http://45.12.75.241:8888")
	log.Printf("[OpenRewrite Dispatcher]   OUTPUT_KEY: %s", fmt.Sprintf("jobs/%s/output.tar", req.JobID))
	log.Printf("[OpenRewrite Dispatcher]   Expected UPLOAD_URL: %s/artifacts/%s", "http://45.12.75.241:8888", fmt.Sprintf("jobs/%s/output.tar", req.JobID))
	log.Printf("[OpenRewrite Dispatcher]   Storage bucket: artifacts")
	
	job := d.createNomadJob(req, nomadJobName)

	// Log the download URL that will be used
	downloadURL := fmt.Sprintf("%s/artifacts/jobs/%s/input.tar", d.seaweedfsURL, req.JobID)
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

	// Wait for evaluation to complete and get allocation ID
	log.Printf("[OpenRewrite Dispatcher] Waiting for evaluation to create allocation...")
	allocationID, err := d.waitForAllocationFromEval(ctx, jobResp.EvalID, req.JobID)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Failed to get allocation from evaluation: %v", err)
		return nil, fmt.Errorf("failed to get allocation from evaluation: %w", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Got allocation ID: %s", allocationID)

	// Wait for job completion using specific allocation
	log.Printf("[OpenRewrite Dispatcher] Waiting for allocation %s to complete", allocationID)
	result, err := d.waitForAllocationCompletion(ctx, allocationID, req.JobID)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Job execution failed: %v", err)
		return nil, fmt.Errorf("job execution failed: %w", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Job completed successfully")

	// Download and extract output (bucket is already 'artifacts', so key should not include 'artifacts/' prefix)
	outputStorageKey := fmt.Sprintf("jobs/%s/output.tar", req.JobID)
	outputTarPath := fmt.Sprintf("/tmp/%s-output.tar", req.JobID)
	defer os.Remove(outputTarPath)

	log.Printf("[OpenRewrite Dispatcher] Downloading output from storage:")
	log.Printf("[OpenRewrite Dispatcher]   Storage key: %s", outputStorageKey)
	log.Printf("[OpenRewrite Dispatcher]   Storage bucket: artifacts")
	log.Printf("[OpenRewrite Dispatcher]   Expected full path: %s/artifacts/%s", d.seaweedfsURL, outputStorageKey)
	log.Printf("[OpenRewrite Dispatcher]   Local path: %s", outputTarPath)

	if err := d.downloadFromStorage(ctx, outputStorageKey, outputTarPath); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Download failed for key=%s: %v", outputStorageKey, err)
		return nil, fmt.Errorf("failed to download output tar: %w", err)
	}
	
	log.Printf("[OpenRewrite Dispatcher] Output download successful")

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
func (d *OpenRewriteDispatcher) createNomadJob(req *OpenRewriteRecipeRequest, jobName string) *api.Job {
	jobID := jobName
	jobType := "batch"
	priority := 50
	datacenters := []string{"dc1"}

	// Create task group
	taskGroup := &api.TaskGroup{
		Name: stringPtr("openrewrite"),
		Tasks: []*api.Task{
			{
				Name:   "openrewrite", // Changed from "transform" to match task group name
				Driver: "docker",
				Config: map[string]interface{}{
					// Use custom OpenRewrite image from registry (now with setup script)
					"image":      fmt.Sprintf("%s/openrewrite-jvm:latest", d.registryURL),
					"force_pull": true, // Force pull to get latest image with setup script
				},
                Env: map[string]string{
                    "JOB_ID":            req.JobID,            // Nomad job ID for storage paths
                    "TRANSFORMATION_ID": req.TransformationID, // UUID from ARF handler
                    "RECIPE":            req.RecipeClass,
                    // Prefer dynamic discovery; leave RECIPE_* empty
                    "RECIPE_GROUP":      "",
                    "RECIPE_ARTIFACT":   "",
                    "RECIPE_VERSION":    "",
                    "SEAWEEDFS_URL":     "http://45.12.75.241:8888",
                    "PLOY_API_URL":      d.apiURL,
                    "MAVEN_CACHE_PATH":  "maven-repository",
                    "DISCOVER_RECIPE":   "true",                                                                   // Let runner handle pack resolution
                    "ARTIFACT_URL":      fmt.Sprintf("%s/artifacts/jobs/%s/input.tar", d.seaweedfsURL, req.JobID), // Full artifact URL
                    "OUTPUT_KEY":        fmt.Sprintf("jobs/%s/output.tar", req.JobID),                             // Key retained for backward compatibility
                    "OUTPUT_URL":        fmt.Sprintf("%s/artifacts/jobs/%s/output.tar", d.seaweedfsURL, req.JobID), // Prefer full upload URL in runner
                },
				Resources: &api.Resources{
					CPU:      intPtr(500),
					MemoryMB: intPtr(2048),
				},
				// Add artifact download/upload tasks
				Artifacts: []*api.TaskArtifact{
					{
						// Download artifact from SeaweedFS
						GetterSource: stringPtr(fmt.Sprintf("%s/artifacts/jobs/%s/input.tar", d.seaweedfsURL, req.JobID)),
						RelativeDest: stringPtr("local/"), // Download to local/ directory
						GetterOptions: map[string]string{
							"archive": "false", // Prevent Nomad from auto-extracting the tar
						},
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

// waitForAllocationFromEval waits for an evaluation to create an allocation
func (d *OpenRewriteDispatcher) waitForAllocationFromEval(ctx context.Context, evalID, jobID string) (string, error) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.After(30 * time.Second) // Should be quick

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for allocation from eval %s", evalID)
		case <-ticker.C:
			// Get allocations created by this evaluation
			allocs, _, err := d.nomadClient.Evaluations().Allocations(evalID, nil)
			if err != nil {
				log.Printf("[OpenRewrite Dispatcher] Warning: Failed to get allocations for eval %s: %v", evalID, err)
				continue
			}

			// Find allocation for our job
			for _, alloc := range allocs {
				if alloc.JobID == jobID {
					log.Printf("[OpenRewrite Dispatcher] Found allocation %s for job %s", alloc.ID, jobID)
					return alloc.ID, nil
				}
			}
		}
	}
}

// waitForAllocationCompletion waits for a specific allocation to complete
func (d *OpenRewriteDispatcher) waitForAllocationCompletion(ctx context.Context, allocationID, jobID string) (*TransformationResult, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Increase timeout to 10 minutes for longer transformations
	timeout := time.After(10 * time.Minute)
	startTime := time.Now()
	lastStatus := ""

	for {
		select {
		case <-ctx.Done():
			log.Printf("[OpenRewrite Dispatcher] Context cancelled while waiting for allocation %s - performing cleanup", allocationID)
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
			log.Printf("[OpenRewrite Dispatcher] Allocation %s timed out after 10 minutes", allocationID)
			return nil, fmt.Errorf("job execution timeout after 10 minutes")

		case <-ticker.C:
			// Get specific allocation info
			alloc, _, err := d.nomadClient.Allocations().Info(allocationID, nil)
			if err != nil {
				log.Printf("[OpenRewrite Dispatcher] ERROR: Failed to get allocation info for %s: %v", allocationID, err)
				continue
			}

			// Log status changes
			currentStatus := alloc.ClientStatus
			if currentStatus != lastStatus {
				log.Printf("[OpenRewrite Dispatcher] Allocation %s status: %s (elapsed: %v)",
					allocationID, currentStatus, time.Since(startTime))
				lastStatus = currentStatus

				// Log task states
				if alloc.TaskStates != nil {
					for taskName, taskState := range alloc.TaskStates {
						log.Printf("[OpenRewrite Dispatcher] Task %s: State=%s, Failed=%v, Restarts=%d",
							taskName, taskState.State, taskState.Failed, taskState.Restarts)
					}
				}
			}

			// Check for completion
			if alloc.ClientStatus == "complete" {
				log.Printf("[OpenRewrite Dispatcher] Allocation %s completed successfully", allocationID)
				// Check task exit codes
				success := true
				for taskName, taskState := range alloc.TaskStates {
					if taskState.Failed {
						// Check if the exit code is actually 0 (success)
						for _, event := range taskState.Events {
							if event.Type == "Terminated" && event.ExitCode == 0 {
								log.Printf("[OpenRewrite Dispatcher] Task %s exited with code 0 (success)", taskName)
								success = true
								break
							}
						}
						if !success {
							log.Printf("[OpenRewrite Dispatcher] Task %s failed", taskName)
							return nil, fmt.Errorf("task %s failed", taskName)
						}
					}
				}
				return &TransformationResult{
					Success:        true,
					ExecutionTime:  time.Since(startTime),
					ChangesApplied: 1, // This should be populated from actual results
				}, nil
			}

			// Check for failure
			if alloc.ClientStatus == "failed" {
				log.Printf("[OpenRewrite Dispatcher] Allocation %s failed", allocationID)
				// Get failure reason from task states
				failureReason := "unknown failure"
				if alloc.TaskStates != nil {
					for taskName, taskState := range alloc.TaskStates {
						for _, event := range taskState.Events {
							if event.Type == "Driver Failure" || event.Type == "Task Setup" || event.DisplayMessage != "" {
								failureReason = fmt.Sprintf("%s: %s", taskName, event.DisplayMessage)
								break
							}
						}
					}
				}
				return nil, fmt.Errorf("allocation failed: %s", failureReason)
			}

			// Check for lost allocation
			if alloc.ClientStatus == "lost" {
				log.Printf("[OpenRewrite Dispatcher] Allocation %s was lost", allocationID)
				return nil, fmt.Errorf("allocation lost")
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
	log.Printf("[OpenRewrite Dispatcher] ===== TAR EXTRACTION START =====")
	log.Printf("[OpenRewrite Dispatcher] Source tar file: %s", tarPath)
	log.Printf("[OpenRewrite Dispatcher] Destination repo: %s", repoPath)

	// Check if tar file exists and get stats
	tarInfo, err := os.Stat(tarPath)
	if err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot stat tar file %s: %v", tarPath, err)
		return fmt.Errorf("tar file not accessible: %w", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Tar file size: %d bytes", tarInfo.Size())

	// Check if destination directory exists, create if needed
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Cannot create destination directory %s: %v", repoPath, err)
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Test tar file integrity first
	log.Printf("[OpenRewrite Dispatcher] Testing tar file integrity...")
	testCmd := fmt.Sprintf("tar -tf %s", tarPath)
	if output, err := executeCommandWithOutput(testCmd); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Tar integrity test failed: %v", err)
		log.Printf("[OpenRewrite Dispatcher] Tar test output: %s", output)
		return fmt.Errorf("tar file integrity test failed: %w", err)
	} else {
		log.Printf("[OpenRewrite Dispatcher] Tar integrity test passed")
		// Log first few entries for debugging
		lines := strings.Split(output, "\n")
		maxLines := 10
		if len(lines) < maxLines {
			maxLines = len(lines)
		}
		log.Printf("[OpenRewrite Dispatcher] Tar contents preview (first %d entries):", maxLines)
		for i := 0; i < maxLines && i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) != "" {
				log.Printf("  %s", strings.TrimSpace(lines[i]))
			}
		}
	}

	// Perform extraction with verbose output
	log.Printf("[OpenRewrite Dispatcher] Performing tar extraction...")
	cmd := fmt.Sprintf("tar -xvf %s -C %s", tarPath, repoPath)
	if output, err := executeCommandWithOutput(cmd); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Tar extraction failed: %v", err)
		log.Printf("[OpenRewrite Dispatcher] Extraction output: %s", output)

		// Additional diagnostics
		log.Printf("[OpenRewrite Dispatcher] === EXTRACTION FAILURE DIAGNOSTICS ===")

		// Check destination permissions
		if destInfo, statErr := os.Stat(repoPath); statErr == nil {
			log.Printf("[OpenRewrite Dispatcher] Destination permissions: %v", destInfo.Mode())
		}

		// Check disk space (simple approach)
		if spaceOutput, spaceErr := executeCommandWithOutput("df -h " + repoPath); spaceErr == nil {
			log.Printf("[OpenRewrite Dispatcher] Disk space: %s", spaceOutput)
		}

		return fmt.Errorf("tar extraction failed with exit code: %w", err)
	} else {
		log.Printf("[OpenRewrite Dispatcher] Extraction completed successfully")
		// Log extraction summary
		extractedLines := strings.Split(output, "\n")
		fileCount := 0
		for _, line := range extractedLines {
			if strings.TrimSpace(line) != "" {
				fileCount++
			}
		}
		log.Printf("[OpenRewrite Dispatcher] Extracted %d files/directories", fileCount)
	}

	log.Printf("[OpenRewrite Dispatcher] ===== TAR EXTRACTION SUCCESS =====")
	return nil
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
	// Note: HTTP URL needs to include bucket prefix since storage client uses bucket + key
	httpURL := fmt.Sprintf("%s/artifacts/%s", d.seaweedfsURL, storageKey)
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
// This function maps recipe class names to appropriate Maven coordinates for OpenRewrite catalogs.
func ParseOpenRewriteRecipeID(recipeID string) (*OpenRewriteRecipeRequest, error) {
	// Validate that the recipe ID looks like a valid OpenRewrite recipe class
	if recipeID == "" {
		return nil, fmt.Errorf("recipe ID cannot be empty")
	}

    // All OpenRewrite recipes should use full class names (e.g., org.openrewrite.java.migrate.UpgradeToJava17)
    // Provide explicit Maven coordinates to ensure recipes are available in the runner without relying on discovery.
    // Mapping:
    //  - org.openrewrite.java.spring.*   → rewrite-spring
    //  - org.openrewrite.java.migrate.*  → rewrite-migrate-java
    //  - org.openrewrite.java.cleanup.*  → rewrite-java
    //  - default                         → rewrite-java
    artifact := "rewrite-java"
    switch {
    case strings.HasPrefix(recipeID, "org.openrewrite.java.spring"):
        artifact = "rewrite-spring"
    case strings.HasPrefix(recipeID, "org.openrewrite.java.migrate"):
        artifact = "rewrite-migrate-java"
    case strings.HasPrefix(recipeID, "org.openrewrite.java.cleanup"):
        artifact = "rewrite-java"
    default:
        artifact = "rewrite-java"
    }

    return &OpenRewriteRecipeRequest{
        RecipeClass:    recipeID,
        RecipeGroup:    "org.openrewrite.recipe",
        RecipeArtifact: artifact,
        RecipeVersion:  "2.20.0",
    }, nil
}

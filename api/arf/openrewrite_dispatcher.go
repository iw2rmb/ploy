package arf

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"net/http"

	"github.com/hashicorp/nomad/api"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
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
	// Install retry transport for resilience on read paths
	config.HttpClient = &http.Client{Transport: orchestration.NewDefaultRetryTransport(nil), Timeout: 60 * time.Second}
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

// ExecuteOpenRewriteRecipe orchestrates an OpenRewrite transformation using Nomad
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
	testKey := fmt.Sprintf("connectivity-test-%d", time.Now().Unix())
	testData := []byte("connectivity-test")
	log.Printf("[OpenRewrite Dispatcher] Testing storage connectivity with key: %s", testKey)
	if err := d.storageClient.Put(ctx, testKey, testData); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Storage connectivity test failed: %v", err)
		return nil, fmt.Errorf("storage not accessible: %w", err)
	}
	log.Printf("[OpenRewrite Dispatcher] Storage connectivity test successful")

	// Clean up test file
	if err := d.storageClient.Delete(ctx, testKey); err != nil {
		log.Printf("[OpenRewrite Dispatcher] WARNING: Failed to delete connectivity test file %s: %v", testKey, err)
	}

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

	// Build HCL spec equivalent to createNomadJob and submit via orchestration facade
    // Build environment for batch job
    env := map[string]string{
        "JOB_ID":            req.JobID,
        "TRANSFORMATION_ID": req.TransformationID,
        "RECIPE":            req.RecipeClass,
        "SEAWEEDFS_URL":     d.seaweedfsURL,
        "PLOY_API_URL":      d.apiURL,
        "MAVEN_CACHE_PATH":  "maven-repository",
        "DISCOVER_RECIPE":   "true",
        "ARTIFACT_URL":      fmt.Sprintf("%s/artifacts/jobs/%s/input.tar", d.seaweedfsURL, req.JobID),
        "OUTPUT_KEY":        fmt.Sprintf("jobs/%s/output.tar", req.JobID),
        "OUTPUT_URL":        fmt.Sprintf("%s/artifacts/jobs/%s/output.tar", d.seaweedfsURL, req.JobID),
    }
    // If explicit Maven coordinates are provided, pass them and disable discovery
    if req.RecipeGroup != "" && req.RecipeArtifact != "" && req.RecipeVersion != "" {
        env["RECIPE_GROUP"] = req.RecipeGroup
        env["RECIPE_ARTIFACT"] = req.RecipeArtifact
        env["RECIPE_VERSION"] = req.RecipeVersion
        env["DISCOVER_RECIPE"] = "false"
    }
	artifactURL := fmt.Sprintf("%s/artifacts/jobs/%s/input.tar", d.seaweedfsURL, req.JobID)
	hcl := orchestration.RenderBatchDockerJobHCL(nomadJobName, "openrewrite", "openrewrite", fmt.Sprintf("%s/openrewrite-jvm:latest", d.registryURL), env, artifactURL)

	tmpFile, err := os.CreateTemp("", "openrewrite-*.hcl")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp job file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString(hcl); err != nil {
		return nil, fmt.Errorf("failed to write job HCL: %w", err)
	}
	tmpFile.Close()

	log.Printf("[OpenRewrite Dispatcher] Submitting job via orchestration")
	if err := orchestration.Submit(tmpFile.Name()); err != nil {
		log.Printf("[OpenRewrite Dispatcher] ERROR: Submit failed: %v", err)
		return nil, fmt.Errorf("failed to submit job: %w", err)
	}

	// Discover allocation using blocking queries to reduce control-plane load
	log.Printf("[OpenRewrite Dispatcher] Waiting for allocation to appear (blocking)...")
	var allocationID string
	var lastIndex uint64
	waitTime := 30 * time.Second
	if v := os.Getenv("NOMAD_BLOCKING_WAIT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			waitTime = d
		}
	}
	allocDeadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(allocDeadline) {
		q := &api.QueryOptions{WaitIndex: lastIndex, WaitTime: waitTime, AllowStale: true}
		allocs, meta, err := d.nomadClient.Jobs().Allocations(req.JobID, false, q)
		if err == nil {
			if meta != nil && meta.LastIndex > 0 {
				lastIndex = meta.LastIndex
			}
			if len(allocs) > 0 {
				allocationID = allocs[0].ID
				break
			}
		}
		// no explicit sleep; blocking query already waited
	}
	if allocationID == "" {
		return nil, fmt.Errorf("timeout waiting for allocation for job %s", req.JobID)
	}

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

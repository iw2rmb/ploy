package arf

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/nomad/api"
)

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
				Env: func() map[string]string {
					env := map[string]string{
						"JOB_ID":            req.JobID,            // Nomad job ID for storage paths
						"TRANSFORMATION_ID": req.TransformationID, // UUID from ARF handler
						"RECIPE":            req.RecipeClass,
						"SEAWEEDFS_URL":     "http://45.12.75.241:8888",
						"PLOY_API_URL":      d.apiURL,
						"MAVEN_CACHE_PATH":  "maven-repository",
						"DISCOVER_RECIPE":   "true",                                                                   // Let runner handle pack resolution (overridden when coords provided)
						"ARTIFACT_URL":      fmt.Sprintf("%s/artifacts/jobs/%s/input.tar", d.seaweedfsURL, req.JobID), // Full artifact URL
						"OUTPUT_KEY":        fmt.Sprintf("jobs/%s/output.tar", req.JobID),                             // Key retained for backward compatibility
						"OUTPUT_URL":        fmt.Sprintf("%s/artifacts/jobs/%s/output.tar", d.seaweedfsURL, req.JobID),
					}
					if req.RecipeGroup != "" && req.RecipeArtifact != "" && req.RecipeVersion != "" {
						env["RECIPE_GROUP"] = req.RecipeGroup
						env["RECIPE_ARTIFACT"] = req.RecipeArtifact
						env["RECIPE_VERSION"] = req.RecipeVersion
						env["DISCOVER_RECIPE"] = "false"
					}
					return env
				}(),
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

// waitForAllocationCompletion waits for an allocation to complete and returns the result
func (d *OpenRewriteDispatcher) waitForAllocationCompletion(ctx context.Context, allocationID, jobID string) (*TransformationResult, error) {
	log.Printf("[OpenRewrite Dispatcher] ===== ALLOCATION MONITORING START =====")
	log.Printf("[OpenRewrite Dispatcher] Allocation ID: %s", allocationID)
	log.Printf("[OpenRewrite Dispatcher] Job ID: %s", jobID)

	// Track allocation state changes
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.After(10 * time.Minute) // Generous timeout for OpenRewrite transformation

	var lastState string
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[OpenRewrite Dispatcher] Context cancelled after %v", time.Since(startTime))
			return nil, ctx.Err()
		case <-timeout:
			elapsed := time.Since(startTime)
			log.Printf("[OpenRewrite Dispatcher] Timeout waiting for allocation completion after %v", elapsed)
			return nil, fmt.Errorf("timeout waiting for allocation completion after %v", elapsed)
		case <-ticker.C:
			// Get allocation status
			alloc, _, err := d.nomadClient.Allocations().Info(allocationID, nil)
			if err != nil {
				log.Printf("[OpenRewrite Dispatcher] ERROR: Failed to get allocation info: %v", err)
				continue
			}

			currentState := alloc.ClientStatus
			if currentState != lastState {
				elapsed := time.Since(startTime)
				log.Printf("[OpenRewrite Dispatcher] Allocation state change after %v: %s → %s", elapsed, lastState, currentState)
				lastState = currentState
			}

			switch currentState {
			case "complete":
				elapsed := time.Since(startTime)
				log.Printf("[OpenRewrite Dispatcher] ===== ALLOCATION COMPLETED SUCCESSFULLY =====")
				log.Printf("[OpenRewrite Dispatcher] Total execution time: %v", elapsed)

				// Check task state for more details
				taskStates := alloc.TaskStates
				if len(taskStates) > 0 {
					log.Printf("[OpenRewrite Dispatcher] Task states:")
					for taskName, taskState := range taskStates {
						log.Printf("[OpenRewrite Dispatcher]   - %s: state=%s, failed=%v", taskName, taskState.State, taskState.Failed)
						if len(taskState.Events) > 0 {
							log.Printf("[OpenRewrite Dispatcher]     Events:")
							for i, event := range taskState.Events {
								log.Printf("[OpenRewrite Dispatcher]       %d. %s: %s", i+1, event.Type, event.DisplayMessage)
							}
						}
					}
				}

				// Create successful result
				result := &TransformationResult{
					Success:       true,
					ExecutionTime: elapsed,
					StartTime:     startTime,
					EndTime:       time.Now(),
				}

				// If we have task states, check if any failed
				allTasksSuccess := true
				var errorMessages []TransformationError
				for _, taskState := range taskStates {
					if taskState.Failed {
						allTasksSuccess = false
						errorMessages = append(errorMessages, TransformationError{
							Message: fmt.Sprintf("task failed: %s", taskState.State),
							Type:    "task_failure",
						})
					}
				}

				if !allTasksSuccess {
					result.Success = false
					result.Errors = errorMessages
					log.Printf("[OpenRewrite Dispatcher] WARNING: Job completed but some tasks failed")
				}

				return result, nil

			case "failed":
				elapsed := time.Since(startTime)
				log.Printf("[OpenRewrite Dispatcher] ===== ALLOCATION FAILED =====")
				log.Printf("[OpenRewrite Dispatcher] Total time before failure: %v", elapsed)

				// Get failure details
				var errorMessages []string
				taskStates := alloc.TaskStates
				if len(taskStates) > 0 {
					log.Printf("[OpenRewrite Dispatcher] Task failure details:")
					for taskName, taskState := range taskStates {
						log.Printf("[OpenRewrite Dispatcher]   - %s: state=%s, failed=%v", taskName, taskState.State, taskState.Failed)
						if len(taskState.Events) > 0 {
							log.Printf("[OpenRewrite Dispatcher]     Events:")
							for i, event := range taskState.Events {
								log.Printf("[OpenRewrite Dispatcher]       %d. %s: %s", i+1, event.Type, event.DisplayMessage)
								if event.Type == "Task Setup" || event.Type == "Driver Failure" || strings.Contains(event.DisplayMessage, "failed") {
									errorMessages = append(errorMessages, event.DisplayMessage)
								}
							}
						}
					}
				}

				// Create error list
				var errors []TransformationError
				if len(errorMessages) > 0 {
					for _, msg := range errorMessages {
						errors = append(errors, TransformationError{
							Message: msg,
							Type:    "job_failure",
						})
					}
				} else {
					errors = []TransformationError{
						{
							Message: "OpenRewrite transformation failed",
							Type:    "job_failure",
						},
					}
				}

				result := &TransformationResult{
					Success:       false,
					ExecutionTime: elapsed,
					StartTime:     startTime,
					EndTime:       time.Now(),
					Errors:        errors,
				}

				return result, nil

			case "running":
				// Continue monitoring
				continue

			case "pending":
				// Continue waiting
				continue

			default:
				log.Printf("[OpenRewrite Dispatcher] Unknown allocation state: %s", currentState)
				continue
			}
		}
	}
}

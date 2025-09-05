package coordination

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
)

// TTLCleanupWorker handles cleanup of preview allocations with TTL
type TTLCleanupWorker struct {
	nomadClient  *nomadapi.Client
	consulClient *api.Client
	logger       *log.Logger
	stopCh       chan struct{}
}

// NewTTLCleanupWorker creates a new TTL cleanup worker
func NewTTLCleanupWorker() *TTLCleanupWorker {
	// Create Nomad client
	nomadConfig := nomadapi.DefaultConfig()
	nomadClient, err := nomadapi.NewClient(nomadConfig)
	if err != nil {
		log.Printf("Failed to create Nomad client: %v", err)
	}

	// Create Consul client
	consulConfig := api.DefaultConfig()
	consulClient, err := api.NewClient(consulConfig)
	if err != nil {
		log.Printf("Failed to create Consul client: %v", err)
	}

	return &TTLCleanupWorker{
		nomadClient:  nomadClient,
		consulClient: consulClient,
		logger:       log.New(os.Stdout, "[ttl-cleanup] ", log.LstdFlags|log.Lshortfile),
		stopCh:       make(chan struct{}),
	}
}

// Run starts the TTL cleanup worker
func (w *TTLCleanupWorker) Run(ctx context.Context) {
	w.logger.Println("Starting TTL cleanup worker")

	ticker := time.NewTicker(5 * time.Minute) // Run every 5 minutes
	defer ticker.Stop()

	// Run immediately on start
	w.performCleanup()

	for {
		select {
		case <-ctx.Done():
			w.logger.Println("TTL cleanup worker stopped by context")
			return
		case <-w.stopCh:
			w.logger.Println("TTL cleanup worker stopped")
			return
		case <-ticker.C:
			w.performCleanup()
		}
	}
}

// Stop stops the TTL cleanup worker
func (w *TTLCleanupWorker) Stop() {
	close(w.stopCh)
}

// performCleanup performs the actual cleanup of expired preview allocations
func (w *TTLCleanupWorker) performCleanup() {
	w.logger.Println("Starting TTL cleanup scan")

	if w.nomadClient == nil {
		w.logger.Println("Nomad client not available, skipping cleanup")
		return
	}

	jobs := w.nomadClient.Jobs()
	jobList, _, err := jobs.List(&nomadapi.QueryOptions{})
	if err != nil {
		w.logger.Printf("Failed to list jobs: %v", err)
		return
	}

	cleaned := 0
	for _, jobStub := range jobList {
		if w.shouldCleanupJob(jobStub) {
			if w.cleanupJob(jobStub.ID) {
				cleaned++
			}
		}
	}

	w.logger.Printf("TTL cleanup completed, cleaned %d jobs", cleaned)
}

// shouldCleanupJob determines if a job should be cleaned up based on TTL
func (w *TTLCleanupWorker) shouldCleanupJob(jobStub *nomadapi.JobListStub) bool {
	// Only clean up preview jobs (they have specific naming pattern)
	if !strings.Contains(jobStub.ID, "-preview-") && !strings.HasPrefix(jobStub.ID, "preview-") {
		return false
	}

	// Get full job details
	job, _, err := w.nomadClient.Jobs().Info(jobStub.ID, &nomadapi.QueryOptions{})
	if err != nil {
		w.logger.Printf("Failed to get job info for %s: %v", jobStub.ID, err)
		return false
	}

	// Check if job has TTL metadata
	ttlStr, ok := job.Meta["ttl"]
	if !ok {
		// Default TTL for preview jobs without explicit TTL
		ttlStr = "1h"
	}

	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		w.logger.Printf("Invalid TTL format for job %s: %s", jobStub.ID, ttlStr)
		return false
	}

	// Check if job has exceeded TTL
	submitTime := time.Unix(0, jobStub.SubmitTime)
	expireTime := submitTime.Add(ttl)

	if time.Now().After(expireTime) {
		w.logger.Printf("Job %s has expired (TTL: %s, Age: %s)",
			jobStub.ID, ttl, time.Since(submitTime))
		return true
	}

	return false
}

// cleanupJob removes an expired job and its associated resources
func (w *TTLCleanupWorker) cleanupJob(jobID string) bool {
	w.logger.Printf("Cleaning up expired job: %s", jobID)

	// Stop the job
	_, _, err := w.nomadClient.Jobs().Deregister(jobID, true, &nomadapi.WriteOptions{})
	if err != nil {
		w.logger.Printf("Failed to stop job %s: %v", jobID, err)
		return false
	}

	// Clean up associated Consul KV entries
	w.cleanupConsulEntries(jobID)

	// Clean up associated services from Consul
	w.cleanupConsulServices(jobID)

	w.logger.Printf("Successfully cleaned up job: %s", jobID)
	return true
}

// cleanupConsulEntries removes Consul KV entries associated with a job
func (w *TTLCleanupWorker) cleanupConsulEntries(jobID string) {
	if w.consulClient == nil {
		return
	}

	kv := w.consulClient.KV()

	// Common patterns for job-related KV entries
	patterns := []string{
		fmt.Sprintf("ploy/jobs/%s/", jobID),
		fmt.Sprintf("ploy/apps/%s/", extractAppName(jobID)),
		fmt.Sprintf("ploy/previews/%s/", jobID),
	}

	for _, pattern := range patterns {
		keys, _, err := kv.List(pattern, &api.QueryOptions{})
		if err != nil {
			w.logger.Printf("Failed to list keys for pattern %s: %v", pattern, err)
			continue
		}

		for _, key := range keys {
			_, err := kv.Delete(key.Key, &api.WriteOptions{})
			if err != nil {
				w.logger.Printf("Failed to delete key %s: %v", key.Key, err)
			} else {
				w.logger.Printf("Deleted KV entry: %s", key.Key)
			}
		}
	}
}

// cleanupConsulServices removes Consul services associated with a job
func (w *TTLCleanupWorker) cleanupConsulServices(jobID string) {
	if w.consulClient == nil {
		return
	}

	catalog := w.consulClient.Catalog()

	// List all services
	services, _, err := catalog.Services(&api.QueryOptions{})
	if err != nil {
		w.logger.Printf("Failed to list services: %v", err)
		return
	}

	// Look for services related to this job
	for serviceName := range services {
		if strings.Contains(serviceName, jobID) ||
			strings.Contains(serviceName, extractAppName(jobID)) {

			// Get service instances
			serviceNodes, _, err := catalog.Service(serviceName, "", &api.QueryOptions{})
			if err != nil {
				w.logger.Printf("Failed to get service nodes for %s: %v", serviceName, err)
				continue
			}

			// Deregister service instances
			agent := w.consulClient.Agent()
			for _, node := range serviceNodes {
				err := agent.ServiceDeregister(node.ServiceID)
				if err != nil {
					w.logger.Printf("Failed to deregister service %s: %v", node.ServiceID, err)
				} else {
					w.logger.Printf("Deregistered service: %s", node.ServiceID)
				}
			}
		}
	}
}

// extractAppName extracts the app name from a job ID
func extractAppName(jobID string) string {
	// Handle various job ID patterns:
	// preview-<app>-<hash>
	// <app>-preview-<hash>
	// <app>-<lane>-<hash>

	parts := strings.Split(jobID, "-")
	if len(parts) >= 2 {
		if parts[0] == "preview" {
			return parts[1]
		}
		return parts[0]
	}

	return jobID
}

// GetCleanupStats returns statistics about TTL cleanup operations
func (w *TTLCleanupWorker) GetCleanupStats() map[string]interface{} {
	if w.nomadClient == nil {
		return map[string]interface{}{
			"enabled": false,
			"error":   "Nomad client not available",
		}
	}

	jobs := w.nomadClient.Jobs()
	jobList, _, err := jobs.List(&nomadapi.QueryOptions{})
	if err != nil {
		return map[string]interface{}{
			"enabled": true,
			"error":   fmt.Sprintf("Failed to list jobs: %v", err),
		}
	}

	previewJobs := 0
	expiredJobs := 0

	for _, jobStub := range jobList {
		if strings.Contains(jobStub.ID, "-preview-") || strings.HasPrefix(jobStub.ID, "preview-") {
			previewJobs++
			if w.shouldCleanupJob(jobStub) {
				expiredJobs++
			}
		}
	}

	return map[string]interface{}{
		"enabled":      true,
		"preview_jobs": previewJobs,
		"expired_jobs": expiredJobs,
		"last_run":     time.Now().Format(time.RFC3339),
	}
}

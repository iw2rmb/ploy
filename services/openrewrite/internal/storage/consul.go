package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/consul/api"
)

// ConsulClient handles job metadata storage in Consul KV
type ConsulClient struct {
	client    *api.Client
	keyPrefix string
}

// JobMetadata represents job information stored in Consul
type JobMetadata struct {
	JobID      string    `json:"job_id"`
	Status     string    `json:"status"`     // pending, running, completed, failed
	Progress   int       `json:"progress"`   // 0-100
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time,omitempty"`
	Error      string    `json:"error,omitempty"`
	Recipe     string    `json:"recipe"`
	DiffURL    string    `json:"diff_url,omitempty"`    // SeaweedFS URL for diff
	ArchiveURL string    `json:"archive_url,omitempty"` // SeaweedFS URL for source archive
}

// NewConsulClient creates a new Consul client for OpenRewrite service
func NewConsulClient(consulAddr string) (*ConsulClient, error) {
	config := api.DefaultConfig()
	if consulAddr != "" {
		config.Address = consulAddr
	}
	
	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}
	
	return &ConsulClient{
		client:    client,
		keyPrefix: "ploy/openrewrite/jobs",
	}, nil
}

// StoreJobMetadata saves job metadata to Consul
func (c *ConsulClient) StoreJobMetadata(metadata JobMetadata) error {
	key := fmt.Sprintf("%s/%s", c.keyPrefix, metadata.JobID)
	
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal job metadata: %w", err)
	}
	
	pair := &api.KVPair{
		Key:   key,
		Value: data,
	}
	
	_, err = c.client.KV().Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to store job metadata: %w", err)
	}
	
	log.Printf("[ConsulClient] Stored metadata for job %s (status: %s)", metadata.JobID, metadata.Status)
	return nil
}

// GetJobMetadata retrieves job metadata from Consul
func (c *ConsulClient) GetJobMetadata(jobID string) (*JobMetadata, error) {
	key := fmt.Sprintf("%s/%s", c.keyPrefix, jobID)
	
	pair, _, err := c.client.KV().Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get job metadata: %w", err)
	}
	
	if pair == nil {
		return nil, fmt.Errorf("job %s not found", jobID)
	}
	
	var metadata JobMetadata
	if err := json.Unmarshal(pair.Value, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job metadata: %w", err)
	}
	
	return &metadata, nil
}

// UpdateJobStatus updates the status of a job
func (c *ConsulClient) UpdateJobStatus(jobID, status string, progress int) error {
	metadata, err := c.GetJobMetadata(jobID)
	if err != nil {
		return fmt.Errorf("failed to get existing metadata: %w", err)
	}
	
	metadata.Status = status
	metadata.Progress = progress
	
	if status == "completed" || status == "failed" {
		metadata.EndTime = time.Now()
		metadata.Progress = 100
	}
	
	return c.StoreJobMetadata(*metadata)
}

// UpdateJobError updates job with error information
func (c *ConsulClient) UpdateJobError(jobID, errorMsg string) error {
	metadata, err := c.GetJobMetadata(jobID)
	if err != nil {
		return fmt.Errorf("failed to get existing metadata: %w", err)
	}
	
	metadata.Status = "failed"
	metadata.Error = errorMsg
	metadata.EndTime = time.Now()
	metadata.Progress = 100
	
	return c.StoreJobMetadata(*metadata)
}

// ListJobs returns a list of all job IDs
func (c *ConsulClient) ListJobs() ([]string, error) {
	keys, _, err := c.client.KV().Keys(c.keyPrefix+"/", "/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	
	var jobIDs []string
	for _, key := range keys {
		// Extract job ID from key (remove prefix)
		jobID := key[len(c.keyPrefix)+1:]
		jobIDs = append(jobIDs, jobID)
	}
	
	return jobIDs, nil
}

// DeleteJob removes a job and its metadata
func (c *ConsulClient) DeleteJob(jobID string) error {
	key := fmt.Sprintf("%s/%s", c.keyPrefix, jobID)
	
	_, err := c.client.KV().Delete(key, nil)
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}
	
	log.Printf("[ConsulClient] Deleted job %s", jobID)
	return nil
}

// Health checks Consul connection
func (c *ConsulClient) Health() error {
	// Try to get a key to verify connection
	_, _, err := c.client.KV().Get("ploy/health", &api.QueryOptions{
		RequireConsistent: true,
	})
	if err != nil {
		return fmt.Errorf("consul health check failed: %w", err)
	}
	
	return nil
}
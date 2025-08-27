package storage

import (
	"fmt"
	"time"
)

// StorageClient provides a unified interface to both Consul and SeaweedFS
type StorageClient struct {
	consul   *ConsulClient
	seaweedfs *SeaweedFSClient
}

// NewStorageClient creates a unified storage client
func NewStorageClient(consulAddr, seaweedAddr string) (*StorageClient, error) {
	consul, err := NewConsulClient(consulAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}
	
	seaweedfs, err := NewSeaweedFSClient(seaweedAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create SeaweedFS client: %w", err)
	}
	
	return &StorageClient{
		consul:    consul,
		seaweedfs: seaweedfs,
	}, nil
}

// CreateJob creates a new job with initial metadata
func (s *StorageClient) CreateJob(jobID, recipe string, archive []byte) (*JobMetadata, error) {
	// Store archive in SeaweedFS
	archiveURL, err := s.seaweedfs.StoreArchive(jobID, archive)
	if err != nil {
		return nil, fmt.Errorf("failed to store archive: %w", err)
	}
	
	// Create initial job metadata
	metadata := JobMetadata{
		JobID:      jobID,
		Status:     "pending",
		Progress:   0,
		StartTime:  time.Now(),
		Recipe:     recipe,
		ArchiveURL: archiveURL,
	}
	
	// Store metadata in Consul
	if err := s.consul.StoreJobMetadata(metadata); err != nil {
		return nil, fmt.Errorf("failed to store job metadata: %w", err)
	}
	
	return &metadata, nil
}

// GetJob retrieves complete job information
func (s *StorageClient) GetJob(jobID string) (*JobMetadata, error) {
	return s.consul.GetJobMetadata(jobID)
}

// UpdateJobStatus updates job status and progress
func (s *StorageClient) UpdateJobStatus(jobID, status string, progress int) error {
	return s.consul.UpdateJobStatus(jobID, status, progress)
}

// CompleteJob marks a job as completed with diff results
func (s *StorageClient) CompleteJob(jobID string, diff []byte) error {
	// Store diff in SeaweedFS
	diffURL, err := s.seaweedfs.StoreDiff(jobID, diff)
	if err != nil {
		return fmt.Errorf("failed to store diff: %w", err)
	}
	
	// Update job metadata
	metadata, err := s.consul.GetJobMetadata(jobID)
	if err != nil {
		return fmt.Errorf("failed to get job metadata: %w", err)
	}
	
	metadata.Status = "completed"
	metadata.Progress = 100
	metadata.EndTime = time.Now()
	metadata.DiffURL = diffURL
	
	return s.consul.StoreJobMetadata(*metadata)
}

// FailJob marks a job as failed with error message
func (s *StorageClient) FailJob(jobID, errorMsg string) error {
	return s.consul.UpdateJobError(jobID, errorMsg)
}

// GetJobDiff retrieves the diff for a completed job
func (s *StorageClient) GetJobDiff(jobID string) ([]byte, error) {
	return s.seaweedfs.GetDiff(jobID)
}

// GetJobArchive retrieves the source archive for a job
func (s *StorageClient) GetJobArchive(jobID string) ([]byte, error) {
	return s.seaweedfs.GetArchive(jobID)
}

// ListJobs returns all job IDs
func (s *StorageClient) ListJobs() ([]string, error) {
	return s.consul.ListJobs()
}

// DeleteJob removes a job and all associated files
func (s *StorageClient) DeleteJob(jobID string) error {
	// Delete files from SeaweedFS
	s.seaweedfs.DeleteFile(fmt.Sprintf("diffs/%s.diff", jobID))
	s.seaweedfs.DeleteFile(fmt.Sprintf("archives/%s.tar.gz", jobID))
	
	// Delete metadata from Consul
	return s.consul.DeleteJob(jobID)
}

// Health checks both storage backends
func (s *StorageClient) Health() error {
	if err := s.consul.Health(); err != nil {
		return fmt.Errorf("consul health check failed: %w", err)
	}
	
	if err := s.seaweedfs.Health(); err != nil {
		return fmt.Errorf("seaweedfs health check failed: %w", err)
	}
	
	return nil
}
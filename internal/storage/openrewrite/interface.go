package openrewrite

// JobStorage defines the unified storage interface for OpenRewrite service
type JobStorage interface {
	// Status operations (Consul KV)
	StoreJobStatus(jobID string, status *JobStatus) error
	GetJobStatus(jobID string) (*JobStatus, error)
	WatchJobStatus(jobID string, index uint64) (*JobStatus, uint64, error)
	
	// Diff operations (SeaweedFS)
	StoreDiff(jobID string, diff []byte) (string, error)
	RetrieveDiff(fileID string) ([]byte, error)
	DeleteDiff(fileID string) error
	
	// Metrics operations (Consul KV)
	StoreMetrics(metrics *Metrics) error
	GetMetrics() (*Metrics, error)
}

// StatusStorage defines operations for job status management
type StatusStorage interface {
	StoreJobStatus(jobID string, status *JobStatus) error
	GetJobStatus(jobID string) (*JobStatus, error)
	WatchJobStatus(jobID string, index uint64) (*JobStatus, uint64, error)
	StoreMetrics(metrics *Metrics) error
	GetMetrics() (*Metrics, error)
}

// DiffStorage defines operations for diff file management
type DiffStorage interface {
	StoreDiff(jobID string, diff []byte) (string, error)
	RetrieveDiff(fileID string) ([]byte, error)
	DeleteDiff(fileID string) error
}
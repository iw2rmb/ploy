package openrewrite

// CompositeStorage implements JobStorage by combining Consul and SeaweedFS
type CompositeStorage struct {
	consul  StatusStorage
	seaweed DiffStorage
}

// NewCompositeStorage creates a composite storage with custom clients
// The actual consul and seaweedfs clients should be created at the application level
// to avoid circular dependencies
func NewCompositeStorage(consul StatusStorage, seaweed DiffStorage) *CompositeStorage {
	return &CompositeStorage{
		consul:  consul,
		seaweed: seaweed,
	}
}

// StoreJobStatus stores job status in Consul KV
func (c *CompositeStorage) StoreJobStatus(jobID string, status *JobStatus) error {
	return c.consul.StoreJobStatus(jobID, status)
}

// GetJobStatus retrieves job status from Consul KV
func (c *CompositeStorage) GetJobStatus(jobID string) (*JobStatus, error) {
	return c.consul.GetJobStatus(jobID)
}

// WatchJobStatus creates a blocking query for status changes in Consul
func (c *CompositeStorage) WatchJobStatus(jobID string, index uint64) (*JobStatus, uint64, error) {
	return c.consul.WatchJobStatus(jobID, index)
}

// StoreDiff stores diff content in SeaweedFS
func (c *CompositeStorage) StoreDiff(jobID string, diff []byte) (string, error) {
	return c.seaweed.StoreDiff(jobID, diff)
}

// RetrieveDiff retrieves diff content from SeaweedFS
func (c *CompositeStorage) RetrieveDiff(fileID string) ([]byte, error) {
	return c.seaweed.RetrieveDiff(fileID)
}

// DeleteDiff deletes diff from SeaweedFS
func (c *CompositeStorage) DeleteDiff(fileID string) error {
	return c.seaweed.DeleteDiff(fileID)
}

// StoreMetrics stores service metrics in Consul KV
func (c *CompositeStorage) StoreMetrics(metrics *Metrics) error {
	return c.consul.StoreMetrics(metrics)
}

// GetMetrics retrieves service metrics from Consul KV
func (c *CompositeStorage) GetMetrics() (*Metrics, error) {
	return c.consul.GetMetrics()
}

// Ensure CompositeStorage implements JobStorage
var _ JobStorage = (*CompositeStorage)(nil)
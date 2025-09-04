package orchestration

import (
    "encoding/json"
    "fmt"
    "time"
)

// JobStatus represents the detailed status of a Nomad job
type JobStatus struct {
    ID          string   `json:"ID"`
    Name        string   `json:"Name"`
    Status      string   `json:"Status"`
    Type        string   `json:"Type"`
    Datacenters []string `json:"Datacenters"`
    Stable      bool     `json:"Stable"`
    Version     int      `json:"Version"`
}

// AllocationStatus represents detailed allocation status
type AllocationStatus struct {
    ID               string                 `json:"ID"`
    ClientStatus     string                 `json:"ClientStatus"`
    DesiredStatus    string                 `json:"DesiredStatus"`
    DeploymentStatus *AllocDeploymentStatus `json:"DeploymentStatus"`
    TaskStates       map[string]*TaskState  `json:"TaskStates"`
}

// AllocDeploymentStatus represents deployment-specific allocation status
type AllocDeploymentStatus struct {
    Healthy   *bool  `json:"Healthy"`
    Timestamp string `json:"Timestamp"`
}

// TaskState represents the state of a task within an allocation
type TaskState struct {
    State         string       `json:"State"`
    Failed        bool         `json:"Failed"`
    StartedAt     string       `json:"StartedAt"`
    FinishedAt    string       `json:"FinishedAt"`
    Events        []*TaskEvent `json:"Events"`
}

// TaskEvent represents an event for a task
type TaskEvent struct {
    Type           string            `json:"Type"`
    Time           int64             `json:"Time"`
    Message        string            `json:"Message"`
    DisplayMessage string            `json:"DisplayMessage"`
    Details        map[string]string `json:"Details"`
}

// HealthMonitor provides basic health queries for Nomad jobs
type HealthMonitor struct { client nomadAdapter }

// nomadAdapter is a small interface wrapper to allow testing/mocking and SDK usage
type nomadAdapter interface {
    ListAllocations(jobID string) ([]*AllocationStatus, error)
    AllocationEndpoint(allocID string) (string, error)
}

// NewHealthMonitor creates a new health monitor instance reading env defaults
func NewHealthMonitor() *HealthMonitor {
    return &HealthMonitor{ client: newSDKNomadAdapter() }
}

// NewHealthMonitorWithClient constructs a monitor with a provided adapter (used in tests)
func NewHealthMonitorWithClient(adapter nomadAdapter) *HealthMonitor { return &HealthMonitor{client: adapter} }

// GetJobStatus fetches the current status of a job
func (h *HealthMonitor) GetJobStatus(jobID string) (*JobStatus, error) {
    // Minimal status derived from allocations list for SDK path
    allocs, err := h.client.ListAllocations(jobID)
    if err != nil { return nil, err }
    status := &JobStatus{ID: jobID, Name: jobID, Status: "unknown"}
    for _, a := range allocs {
        if a.ClientStatus == "running" { status.Status = "running"; break }
        status.Status = a.ClientStatus
    }
    return status, nil
}

// GetJobAllocations fetches all allocations for a job
func (h *HealthMonitor) GetJobAllocations(jobID string) ([]*AllocationStatus, error) { return h.client.ListAllocations(jobID) }

// IsJobHealthy returns true if at least one running allocation is present
func (h *HealthMonitor) IsJobHealthy(jobID string) bool {
    allocs, err := h.GetJobAllocations(jobID)
    if err != nil {
        return false
    }
    for _, a := range allocs {
        if a.ClientStatus == "running" {
            // consider deployment status if provided
            if a.DeploymentStatus != nil && a.DeploymentStatus.Healthy != nil {
                if *a.DeploymentStatus.Healthy {
                    return true
                }
            } else {
                return true
            }
        }
    }
    return false
}

// GetJobEndpoint attempts to discover an endpoint for a running allocation (http://IP:DynamicPort)
func (h *HealthMonitor) GetJobEndpoint(jobID string) (string, error) {
    allocs, err := h.GetJobAllocations(jobID)
    if err != nil {
        return "", err
    }
    for _, a := range allocs {
        if a.ClientStatus == "running" {
            // fetch detailed allocation to inspect resources
            endpoint, err := h.getAllocationEndpoint(a.ID)
            if err == nil && endpoint != "" {
                return endpoint, nil
            }
        }
    }
    return "", fmt.Errorf("no running allocation found for job %s", jobID)
}

// WaitForHealthyAllocations waits until at least minHealthy allocations are running/healthy for the job.
func (h *HealthMonitor) WaitForHealthyAllocations(jobID string, minHealthy int, timeout time.Duration) error {
    if minHealthy <= 0 { minHealthy = 1 }
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        allocs, err := h.GetJobAllocations(jobID)
        if err != nil {
            time.Sleep(2 * time.Second)
            continue
        }
        healthy := 0
        for _, a := range allocs {
            if a.ClientStatus == "running" {
                if a.DeploymentStatus != nil && a.DeploymentStatus.Healthy != nil {
                    if *a.DeploymentStatus.Healthy { healthy++ }
                } else {
                    healthy++
                }
            }
        }
        if healthy >= minHealthy { return nil }
        time.Sleep(2 * time.Second)
    }
    return fmt.Errorf("timeout waiting for %d healthy allocations for job %s", minHealthy, jobID)
}

// getAllocationEndpoint fetches allocation details and extracts IP:port
func (h *HealthMonitor) getAllocationEndpoint(allocID string) (string, error) { return h.client.AllocationEndpoint(allocID) }

func getenv(k, d string) string { if v := getEnv(k); v != "" { return v }; return d }

// getEnv is split for testability
var getEnv = func(k string) string { return defaultGetenv(k) }

func defaultGetenv(k string) string { return lookupEnv(k) }

// lookupEnv wraps standard library to simplify testing
var lookupEnv = func(k string) string {
    return func(key string) string {
        // defer to stdlib without importing here to keep this file focused
        return mapGetenv(key)
    }(k)
}

// mapGetenv is replaced at build via another file (stub) to call os.Getenv

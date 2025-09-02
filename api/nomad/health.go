package nomad

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DeploymentStatus represents the status of a Nomad deployment
type DeploymentStatus struct {
	ID                string                      `json:"ID"`
	JobID             string                      `json:"JobID"`
	Status            string                      `json:"Status"`
	StatusDescription string                      `json:"StatusDescription"`
	TaskGroups        map[string]*TaskGroupStatus `json:"TaskGroups"`
}

// TaskGroupStatus represents the status of a task group in a deployment
type TaskGroupStatus struct {
	DesiredTotal    int `json:"DesiredTotal"`
	PlacedAllocs    int `json:"PlacedAllocs"`
	HealthyAllocs   int `json:"HealthyAllocs"`
	UnhealthyAllocs int `json:"UnhealthyAllocs"`
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
	State      string       `json:"State"`
	Failed     bool         `json:"Failed"`
	StartedAt  string       `json:"StartedAt"`
	FinishedAt string       `json:"FinishedAt"`
	Events     []*TaskEvent `json:"Events"`
}

// TaskEvent represents an event for a task
type TaskEvent struct {
	Type           string            `json:"Type"`
	Time           int64             `json:"Time"`
	Message        string            `json:"Message"`
	DisplayMessage string            `json:"DisplayMessage"`
	Details        map[string]string `json:"Details"`
}

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

// ServiceHealth represents Consul service health check status
type ServiceHealth struct {
	ServiceName string `json:"ServiceName"`
	CheckID     string `json:"CheckID"`
	Status      string `json:"Status"`
	Output      string `json:"Output"`
}

// HealthMonitor provides comprehensive health monitoring for Nomad jobs
type HealthMonitor struct {
	nomadAddr  string
	consulAddr string
	httpClient *http.Client
}

// NewHealthMonitor creates a new health monitor instance
func NewHealthMonitor() *HealthMonitor {
	return &HealthMonitor{
		nomadAddr:  getenv("NOMAD_ADDR", "http://127.0.0.1:4646"),
		consulAddr: getenv("CONSUL_ADDR", "http://127.0.0.1:8500"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// MonitorDeployment monitors a deployment until it succeeds, fails, or times out
func (h *HealthMonitor) MonitorDeployment(deploymentID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := h.GetDeploymentStatus(deploymentID)
		if err != nil {
			fmt.Printf("Error fetching deployment status: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}

		fmt.Printf("Deployment %s: Status=%s, Description=%s\n",
			deploymentID, status.Status, status.StatusDescription)

		// Check deployment status
		switch status.Status {
		case "successful":
			return nil
		case "failed", "cancelled":
			return fmt.Errorf("deployment %s: %s", status.Status, status.StatusDescription)
		case "running":
			// Print task group progress
			for name, tg := range status.TaskGroups {
				fmt.Printf("  Task Group %s: %d/%d healthy, %d unhealthy\n",
					name, tg.HealthyAllocs, tg.DesiredTotal, tg.UnhealthyAllocs)
			}
		}

		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("deployment monitoring timed out after %v", timeout)
}

// GetDeploymentStatus fetches the current status of a deployment
func (h *HealthMonitor) GetDeploymentStatus(deploymentID string) (*DeploymentStatus, error) {
	url := fmt.Sprintf("%s/v1/deployment/%s", h.nomadAddr, deploymentID)

	resp, err := h.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var status DeploymentStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode deployment status: %w", err)
	}

	return &status, nil
}

// WaitForHealthyAllocations waits for allocations to become healthy
func (h *HealthMonitor) WaitForHealthyAllocations(jobID string, minHealthy int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		allocations, err := h.GetJobAllocations(jobID)
		if err != nil {
			fmt.Printf("Error fetching allocations: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}

		healthyCount := 0
		failedCount := 0
		pendingCount := 0

		for _, alloc := range allocations {
			switch alloc.ClientStatus {
			case "running":
				// Check if allocation is actually healthy
				if h.isAllocationHealthy(alloc) {
					healthyCount++
				}
			case "failed", "lost":
				failedCount++
				// Log failure details
				h.logAllocationFailure(alloc)
			case "pending":
				pendingCount++
			}
		}

		fmt.Printf("Job %s: %d healthy, %d pending, %d failed allocations\n",
			jobID, healthyCount, pendingCount, failedCount)

		if healthyCount >= minHealthy {
			return nil
		}

		// If too many failures, abort early
		if failedCount > 3 {
			return fmt.Errorf("too many failed allocations (%d)", failedCount)
		}

		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("timeout waiting for %d healthy allocations", minHealthy)
}

// GetJobAllocations fetches all allocations for a job
func (h *HealthMonitor) GetJobAllocations(jobID string) ([]*AllocationStatus, error) {
	url := fmt.Sprintf("%s/v1/job/%s/allocations", h.nomadAddr, jobID)

	resp, err := h.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch allocations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var allocations []*AllocationStatus
	if err := json.NewDecoder(resp.Body).Decode(&allocations); err != nil {
		return nil, fmt.Errorf("failed to decode allocations: %w", err)
	}

	return allocations, nil
}

// CheckServiceHealth checks the health of services registered in Consul
func (h *HealthMonitor) CheckServiceHealth(serviceName string) ([]*ServiceHealth, error) {
	url := fmt.Sprintf("%s/v1/health/checks/%s", h.consulAddr, serviceName)

	resp, err := h.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch service health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Service not yet registered in Consul
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var checks []*ServiceHealth
	if err := json.NewDecoder(resp.Body).Decode(&checks); err != nil {
		return nil, fmt.Errorf("failed to decode service health: %w", err)
	}

	return checks, nil
}

// GetJobStatus fetches the current status of a job
func (h *HealthMonitor) GetJobStatus(jobID string) (*JobStatus, error) {
	url := fmt.Sprintf("%s/v1/job/%s", h.nomadAddr, jobID)

	resp, err := h.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch job status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("job %s not found", jobID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var status JobStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode job status: %w", err)
	}

	return &status, nil
}

// WaitForServiceHealth waits for a service to become healthy in Consul
func (h *HealthMonitor) WaitForServiceHealth(serviceName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		checks, err := h.CheckServiceHealth(serviceName)
		if err != nil {
			fmt.Printf("Error checking service health: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if len(checks) == 0 {
			// Service not yet registered
			time.Sleep(2 * time.Second)
			continue
		}

		allHealthy := true
		for _, check := range checks {
			if check.Status != "passing" {
				allHealthy = false
				fmt.Printf("Service %s check %s: %s - %s\n",
					serviceName, check.CheckID, check.Status, check.Output)
			}
		}

		if allHealthy {
			return nil
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for service %s to become healthy", serviceName)
}

// isAllocationHealthy checks if an allocation is truly healthy
func (h *HealthMonitor) isAllocationHealthy(alloc *AllocationStatus) bool {
	// Check deployment health status
	if alloc.DeploymentStatus != nil && alloc.DeploymentStatus.Healthy != nil {
		return *alloc.DeploymentStatus.Healthy
	}

	// Check task states
	for _, taskState := range alloc.TaskStates {
		if taskState.State != "running" || taskState.Failed {
			return false
		}
	}

	return alloc.ClientStatus == "running"
}

// logAllocationFailure logs detailed information about a failed allocation
func (h *HealthMonitor) logAllocationFailure(alloc *AllocationStatus) {
	fmt.Printf("Failed allocation %s:\n", alloc.ID)

	for taskName, taskState := range alloc.TaskStates {
		if taskState.Failed || taskState.State == "dead" {
			fmt.Printf("  Task %s failed: %s\n", taskName, taskState.State)

			// Log recent events
			for _, event := range taskState.Events {
				if event.Type == "Driver Failure" || event.Type == "Task Setup" ||
					event.Type == "Terminated" || event.Type == "Not Restarting" {
					fmt.Printf("    %s: %s\n", event.Type, event.DisplayMessage)
					if event.Message != "" {
						fmt.Printf("      Details: %s\n", event.Message)
					}
				}
			}
		}
	}
}

// GetAllocationDetails fetches detailed information about an allocation
func (h *HealthMonitor) GetAllocationDetails(allocID string) (*AllocationStatus, error) {
	url := fmt.Sprintf("%s/v1/allocation/%s", h.nomadAddr, allocID)

	resp, err := h.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch allocation details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var alloc AllocationStatus
	if err := json.NewDecoder(resp.Body).Decode(&alloc); err != nil {
		return nil, fmt.Errorf("failed to decode allocation: %w", err)
	}

	return &alloc, nil
}

// MonitorJobHealth provides comprehensive health monitoring for a job
func (h *HealthMonitor) MonitorJobHealth(jobID string, expectedCount int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	// First, wait for job to be registered
	for time.Now().Before(deadline) {
		jobStatus, err := h.GetJobStatus(jobID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				time.Sleep(1 * time.Second)
				continue
			}
			return fmt.Errorf("failed to get job status: %w", err)
		}

		fmt.Printf("Job %s status: %s (version %d)\n", jobID, jobStatus.Status, jobStatus.Version)
		break
	}

	// Monitor allocations
	return h.WaitForHealthyAllocations(jobID, expectedCount, timeout)
}

package orchestration

import (
    "encoding/json"
    "fmt"
    "net/http"
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
type HealthMonitor struct {
    nomadAddr  string
    httpClient *http.Client
}

// NewHealthMonitor creates a new health monitor instance reading env defaults
func NewHealthMonitor() *HealthMonitor {
    return &HealthMonitor{
        nomadAddr:  getenv("NOMAD_ADDR", "http://127.0.0.1:4646"),
        httpClient: &http.Client{Timeout: 10 * time.Second},
    }
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

// getAllocationEndpoint fetches allocation details and extracts IP:port
func (h *HealthMonitor) getAllocationEndpoint(allocID string) (string, error) {
    url := fmt.Sprintf("%s/v1/allocation/%s", h.nomadAddr, allocID)
    resp, err := h.httpClient.Get(url)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    }
    var allocData map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&allocData); err != nil {
        return "", err
    }
    // Traverse Resources -> Networks[0] -> IP, DynamicPorts[0].Value
    if resources, ok := allocData["Resources"].(map[string]interface{}); ok {
        if networks, ok := resources["Networks"].([]interface{}); ok && len(networks) > 0 {
            if network, ok := networks[0].(map[string]interface{}); ok {
                if ip, ok := network["IP"].(string); ok {
                    if ports, ok := network["DynamicPorts"].([]interface{}); ok && len(ports) > 0 {
                        if port, ok := ports[0].(map[string]interface{}); ok {
                            if portNum, ok := port["Value"].(float64); ok {
                                return fmt.Sprintf("http://%s:%.0f", ip, portNum), nil
                            }
                        }
                    }
                }
            }
        }
    }
    return "", fmt.Errorf("allocation %s has no endpoint", allocID)
}

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

package orchestration

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

// ServiceHealth represents Consul service health check status
type ServiceHealth struct {
    ServiceName string `json:"ServiceName"`
    CheckID     string `json:"CheckID"`
    Status      string `json:"Status"`
    Output      string `json:"Output"`
}

// ConsulHealth provides service health queries via Consul HTTP API
type ConsulHealth struct {
    consulAddr string
    httpClient *http.Client
}

// NewConsulHealth returns a health client using CONSUL_ADDR or default
func NewConsulHealth() *ConsulHealth {
    return &ConsulHealth{
        consulAddr: getenv("CONSUL_ADDR", "http://127.0.0.1:8500"),
        httpClient: &http.Client{Timeout: 10 * time.Second},
    }
}

// CheckServiceHealth gets health checks for a service
func (h *ConsulHealth) CheckServiceHealth(serviceName string) ([]*ServiceHealth, error) {
    url := fmt.Sprintf("%s/v1/health/checks/%s", h.consulAddr, serviceName)
    resp, err := h.httpClient.Get(url)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch service health: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode == http.StatusNotFound {
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

// WaitForServiceHealth waits until all checks are passing or timeout
func (h *ConsulHealth) WaitForServiceHealth(serviceName string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        checks, err := h.CheckServiceHealth(serviceName)
        if err != nil {
            time.Sleep(2 * time.Second)
            continue
        }
        if len(checks) == 0 {
            time.Sleep(2 * time.Second)
            continue
        }
        allHealthy := true
        for _, c := range checks {
            if c.Status != "passing" {
                allHealthy = false
                break
            }
        }
        if allHealthy { return nil }
        time.Sleep(2 * time.Second)
    }
    return fmt.Errorf("timeout waiting for service %s to be healthy", serviceName)
}


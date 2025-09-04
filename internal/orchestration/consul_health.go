package orchestration

import (
    "fmt"
    "time"

    consulapi "github.com/hashicorp/consul/api"
    "github.com/iw2rmb/ploy/internal/utils"
)

// ServiceHealth represents Consul service health check status
type ServiceHealth struct {
    ServiceName string `json:"ServiceName"`
    CheckID     string `json:"CheckID"`
    Status      string `json:"Status"`
    Output      string `json:"Output"`
}

// ConsulHealth provides service health queries via Consul HTTP API
type consulAdapter interface { Checks(service string) ([]*ServiceHealth, error) }

type ConsulHealth struct { client consulAdapter }

// NewConsulHealth returns a health client using CONSUL_ADDR or default
func NewConsulHealth() *ConsulHealth { return &ConsulHealth{client: newConsulSDKAdapter()} }

// NewConsulHealthWithClient constructs a Consul health with provided adapter (tests)
func NewConsulHealthWithClient(adapter consulAdapter) *ConsulHealth { return &ConsulHealth{client: adapter} }

// CheckServiceHealth gets health checks for a service
func (h *ConsulHealth) CheckServiceHealth(serviceName string) ([]*ServiceHealth, error) { return h.client.Checks(serviceName) }

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

// SDK adapter
type consulSDKAdapter struct { client *consulapi.Client }

func newConsulSDKAdapter() *consulSDKAdapter {
    cfg := consulapi.DefaultConfig()
    if addr := utils.Getenv("CONSUL_ADDR", ""); addr != "" { cfg.Address = addr }
    c, _ := consulapi.NewClient(cfg)
    return &consulSDKAdapter{client: c}
}

func (a *consulSDKAdapter) Checks(service string) ([]*ServiceHealth, error) {
    if a.client == nil { return nil, fmt.Errorf("consul client unavailable") }
    checks, _, err := a.client.Health().Checks(service, nil)
    if err != nil { return nil, err }
    out := make([]*ServiceHealth, 0, len(checks))
    for _, hc := range checks {
        out = append(out, &ServiceHealth{ServiceName: hc.ServiceName, CheckID: hc.CheckID, Status: hc.Status, Output: hc.Output})
    }
    return out, nil
}

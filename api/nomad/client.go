package nomad

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type alloc struct {
	ID           string `json:"ID"`
	ClientStatus string `json:"ClientStatus"`
}

type serviceChecks struct {
	Status string `json:"Status"`
}

func WaitHealthy(jobName string, timeout time.Duration) error {
	// Use the enhanced health monitoring
	monitor := NewHealthMonitor()

	// Wait for at least 1 healthy allocation by default
	// This maintains backward compatibility
	return monitor.WaitForHealthyAllocations(jobName, 1, timeout)
}

func GetJobEndpoint(jobName string) (string, error) {
	addr := getenv("NOMAD_ADDR", "http://127.0.0.1:4646")
	client := &http.Client{Timeout: 5 * time.Second}

	u := fmt.Sprintf("%s/v1/job/%s/allocations", addr, jobName)
	resp, err := client.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var allocs []alloc
	if err := json.NewDecoder(resp.Body).Decode(&allocs); err != nil {
		return "", err
	}

	for _, a := range allocs {
		if a.ClientStatus == "running" {
			allocResp, err := client.Get(fmt.Sprintf("%s/v1/allocation/%s", addr, a.ID))
			if err != nil {
				continue
			}
			defer allocResp.Body.Close()

			var allocData map[string]interface{}
			if err := json.NewDecoder(allocResp.Body).Decode(&allocData); err != nil {
				continue
			}

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
		}
	}

	return "", fmt.Errorf("no running allocation found for job %s", jobName)
}

func IsJobHealthy(jobName string) bool {
	return WaitHealthy(jobName, 1*time.Second) == nil
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

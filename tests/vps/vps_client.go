package vps

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// VPSClient provides methods to interact with the VPS for testing
type VPSClient struct {
	host string
}

// NewVPSClient creates a new VPS client for the given host
func NewVPSClient(host string) *VPSClient {
	return &VPSClient{
		host: host,
	}
}

// RunCommand executes a command on the VPS via SSH
func (c *VPSClient) RunCommand(command string) (string, error) {
	cmd := exec.Command("ssh", "-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=no", fmt.Sprintf("root@%s", c.host), command)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// CheckServiceHealth checks if a service is healthy on the VPS
func (c *VPSClient) CheckServiceHealth(service string) (bool, error) {
	var checkCmd string

	switch service {
	case "consul":
		checkCmd = `su - ploy -c "curl -f http://localhost:8500/v1/status/leader"`
	case "nomad":
		checkCmd = `su - ploy -c "/opt/hashicorp/bin/nomad-job-manager.sh status"`
	case "seaweedfs-master":
		checkCmd = `su - ploy -c "curl -f http://localhost:9333/cluster/status"`
	case "seaweedfs-filer":
		checkCmd = `su - ploy -c "curl -f http://localhost:8888/"`
	default:
		return false, fmt.Errorf("unknown service: %s", service)
	}

	output, err := c.RunCommand(checkCmd)
	if err != nil {
		return false, fmt.Errorf("service health check failed: %s, error: %v", output, err)
	}

	// Simple check - if command succeeded without error, service is healthy
	return !strings.Contains(strings.ToLower(output), "error") &&
		!strings.Contains(strings.ToLower(output), "fail"), nil
}

// CheckServiceEndpoint checks if a service endpoint is reachable via HTTP
func (c *VPSClient) CheckServiceEndpoint(endpoint string) (bool, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

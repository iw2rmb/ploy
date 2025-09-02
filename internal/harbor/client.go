package harbor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client represents a Harbor API client
type Client struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

// Project represents a Harbor project
type Project struct {
	Name     string            `json:"name"`
	Public   bool              `json:"public,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Storage  int64             `json:"storage_limit,omitempty"`
}

// Repository represents a Harbor repository
type Repository struct {
	Name          string    `json:"name"`
	ProjectID     int       `json:"project_id"`
	Description   string    `json:"description,omitempty"`
	PullCount     int       `json:"pull_count"`
	ArtifactCount int       `json:"artifact_count"`
	UpdateTime    time.Time `json:"update_time"`
}

// ScanReport represents a Harbor vulnerability scan report
type ScanReport struct {
	GeneratedAt     time.Time       `json:"generated_at"`
	Scanner         Scanner         `json:"scanner"`
	Severity        string          `json:"severity"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	Summary         ScanSummary     `json:"summary"`
}

// Scanner represents the scanner information
type Scanner struct {
	Name    string `json:"name"`
	Vendor  string `json:"vendor"`
	Version string `json:"version"`
}

// Vulnerability represents a single vulnerability
type Vulnerability struct {
	ID          string            `json:"id"`
	Package     string            `json:"package"`
	Version     string            `json:"version"`
	Severity    string            `json:"severity"`
	Description string            `json:"description"`
	FixVersion  string            `json:"fix_version,omitempty"`
	Links       []string          `json:"links,omitempty"`
	CVSS        map[string]string `json:"cvss,omitempty"`
}

// ScanSummary represents vulnerability scan summary
type ScanSummary struct {
	Total    int            `json:"total"`
	Fixable  int            `json:"fixable"`
	Summary  map[string]int `json:"summary"`
	Override map[string]int `json:"override,omitempty"`
}

// Robot represents a Harbor robot account
type Robot struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Level       string       `json:"level"`
	Duration    int64        `json:"duration"`
	Permissions []Permission `json:"permissions"`
}

// Permission represents robot account permissions
type Permission struct {
	Kind      string   `json:"kind"`
	Namespace string   `json:"namespace"`
	Access    []Access `json:"access"`
}

// Access represents specific access permissions
type Access struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

// NewClient creates a new Harbor API client
func NewClient(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  baseURL,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateProject creates a new project in Harbor
func (c *Client) CreateProject(project *Project) error {
	data, err := json.Marshal(project)
	if err != nil {
		return fmt.Errorf("failed to marshal project: %w", err)
	}
	
	req, err := http.NewRequest("POST", 
		fmt.Sprintf("%s/api/v2.0/projects", c.baseURL),
		bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()
	
	// 201 Created or 409 Conflict (already exists)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("failed to create project: status %s", resp.Status)
	}
	
	return nil
}

// GetProject retrieves project information
func (c *Client) GetProject(projectName string) (*Project, error) {
	req, err := http.NewRequest("GET", 
		fmt.Sprintf("%s/api/v2.0/projects/%s", c.baseURL, projectName), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.SetBasicAuth(c.username, c.password)
	
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get project: status %s", resp.Status)
	}
	
	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return nil, fmt.Errorf("failed to decode project: %w", err)
	}
	
	return &project, nil
}

// TriggerScan triggers a vulnerability scan for an artifact
func (c *Client) TriggerScan(projectName, repository, tag string) error {
	url := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s/scan",
		c.baseURL, projectName, repository, tag)
	
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create scan request: %w", err)
	}
	
	req.SetBasicAuth(c.username, c.password)
	
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute scan request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("failed to trigger scan: status %s", resp.Status)
	}
	
	return nil
}

// GetScanReport retrieves the vulnerability scan report
func (c *Client) GetScanReport(projectName, repository, tag string) (*ScanReport, error) {
	url := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s/scan",
		c.baseURL, projectName, repository, tag)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.SetBasicAuth(c.username, c.password)
	
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get scan report: status %s", resp.Status)
	}
	
	var report ScanReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, fmt.Errorf("failed to decode scan report: %w", err)
	}
	
	return &report, nil
}

// DeleteRepository deletes a repository from Harbor
func (c *Client) DeleteRepository(projectName, repository string) error {
	url := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s",
		c.baseURL, projectName, repository)
	
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}
	
	req.SetBasicAuth(c.username, c.password)
	
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute delete request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete repository: status %s", resp.Status)
	}
	
	return nil
}

// CreateRobot creates a robot account for automated operations
func (c *Client) CreateRobot(robot *Robot) (map[string]interface{}, error) {
	data, err := json.Marshal(robot)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal robot: %w", err)
	}
	
	req, err := http.NewRequest("POST", 
		fmt.Sprintf("%s/api/v2.0/robots", c.baseURL),
		bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()
	
	// 201 Created or 409 Conflict (already exists)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return nil, fmt.Errorf("failed to create robot: status %s", resp.Status)
	}
	
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode robot response: %w", err)
	}
	
	return result, nil
}

// Health checks Harbor health status
func (c *Client) Health() error {
	req, err := http.NewRequest("GET", 
		fmt.Sprintf("%s/api/v2.0/health", c.baseURL), nil)
	if err != nil {
		return fmt.Errorf("failed to create health request: %w", err)
	}
	
	req.SetBasicAuth(c.username, c.password)
	
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute health request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Harbor is unhealthy: status %s", resp.Status)
	}
	
	return nil
}

// WaitForScanComplete waits for a vulnerability scan to complete
func (c *Client) WaitForScanComplete(projectName, repository, tag string, timeout time.Duration) (*ScanReport, error) {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		report, err := c.GetScanReport(projectName, repository, tag)
		if err != nil {
			// Check if scan is still running
			if err.Error() == "failed to get scan report: status 404 Not Found" {
				time.Sleep(5 * time.Second)
				continue
			}
			return nil, err
		}
		
		// If we got a report, scan is complete
		return report, nil
	}
	
	return nil, fmt.Errorf("scan did not complete within timeout of %v", timeout)
}
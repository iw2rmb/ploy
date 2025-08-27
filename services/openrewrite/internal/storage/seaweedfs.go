package storage

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// SeaweedFSClient handles file storage for OpenRewrite service
type SeaweedFSClient struct {
	masterURL  string
	filerURL   string
	collection string
	timeout    time.Duration
	httpClient *http.Client
}

// NewSeaweedFSClient creates a new SeaweedFS client for file storage
func NewSeaweedFSClient(masterAddr string) (*SeaweedFSClient, error) {
	if masterAddr == "" {
		return nil, fmt.Errorf("seaweedfs master address is required")
	}
	
	// Extract filer URL from master (typically master:9333, filer:8888)
	// For simplicity, assume filer is on same host but port 8888
	filerURL := "http://" + masterAddr
	if masterAddr == "seaweedfs.service.consul:9333" {
		filerURL = "http://seaweedfs.service.consul:8888"
	} else {
		// For other addresses, replace port 9333 with 8888
		filerURL = "http://" + masterAddr[:len(masterAddr)-4] + "8888"
	}
	
	masterURL := "http://" + masterAddr
	
	timeout := 5 * time.Minute // Long timeout for large files
	
	return &SeaweedFSClient{
		masterURL:  masterURL,
		filerURL:   filerURL,
		collection: "openrewrite",
		timeout:    timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// StoreDiff stores a transformation diff in SeaweedFS
func (c *SeaweedFSClient) StoreDiff(jobID string, diff []byte) (string, error) {
	key := fmt.Sprintf("diffs/%s.diff", jobID)
	
	url := fmt.Sprintf("%s/%s/%s?collection=%s", c.filerURL, c.collection, key, c.collection)
	
	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	
	// Add file field
	fileWriter, err := writer.CreateFormFile("file", fmt.Sprintf("%s.diff", jobID))
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	
	_, err = fileWriter.Write(diff)
	if err != nil {
		return "", fmt.Errorf("failed to write diff data: %w", err)
	}
	
	writer.Close()
	
	// Make the request
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload diff: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload failed: %s", resp.Status)
	}
	
	// Return the URL where the diff can be retrieved
	diffURL := fmt.Sprintf("%s/%s/%s", c.filerURL, c.collection, key)
	return diffURL, nil
}

// StoreArchive stores a source code archive in SeaweedFS
func (c *SeaweedFSClient) StoreArchive(jobID string, archive []byte) (string, error) {
	key := fmt.Sprintf("archives/%s.tar.gz", jobID)
	
	url := fmt.Sprintf("%s/%s/%s?collection=%s", c.filerURL, c.collection, key, c.collection)
	
	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	
	// Add file field
	fileWriter, err := writer.CreateFormFile("file", fmt.Sprintf("%s.tar.gz", jobID))
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	
	_, err = fileWriter.Write(archive)
	if err != nil {
		return "", fmt.Errorf("failed to write archive data: %w", err)
	}
	
	writer.Close()
	
	// Make the request
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload archive: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload failed: %s", resp.Status)
	}
	
	// Return the URL where the archive can be retrieved
	archiveURL := fmt.Sprintf("%s/%s/%s", c.filerURL, c.collection, key)
	return archiveURL, nil
}

// GetDiff retrieves a diff from SeaweedFS
func (c *SeaweedFSClient) GetDiff(jobID string) ([]byte, error) {
	key := fmt.Sprintf("diffs/%s.diff", jobID)
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, c.collection, key)
	
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("diff not found: %s", resp.Status)
	}
	
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read diff data: %w", err)
	}
	
	return data, nil
}

// GetArchive retrieves an archive from SeaweedFS
func (c *SeaweedFSClient) GetArchive(jobID string) ([]byte, error) {
	key := fmt.Sprintf("archives/%s.tar.gz", jobID)
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, c.collection, key)
	
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get archive: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archive not found: %s", resp.Status)
	}
	
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive data: %w", err)
	}
	
	return data, nil
}

// DeleteFile removes a file from SeaweedFS
func (c *SeaweedFSClient) DeleteFile(key string) error {
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, c.collection, key)
	
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete failed: %s", resp.Status)
	}
	
	return nil
}

// Health checks SeaweedFS connection
func (c *SeaweedFSClient) Health() error {
	// Check master health
	resp, err := c.httpClient.Get(c.masterURL + "/status")
	if err != nil {
		return fmt.Errorf("master health check failed: %w", err)
	}
	resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("master unhealthy, status: %d", resp.StatusCode)
	}
	
	// Check filer health
	resp, err = c.httpClient.Get(c.filerURL + "/")
	if err != nil {
		return fmt.Errorf("filer health check failed: %w", err)
	}
	resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("filer unhealthy, status: %d", resp.StatusCode)
	}
	
	return nil
}
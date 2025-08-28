package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SeaweedFSConfig represents SeaweedFS-specific configuration
type SeaweedFSConfig struct {
	Master      string `yaml:"master"`      // master server address (e.g., "localhost:9333")
	Filer       string `yaml:"filer"`       // filer server address (e.g., "localhost:8888")
	Collection  string `yaml:"collection"`  // collection name for artifacts
	Replication string `yaml:"replication"` // replication strategy (e.g., "001")
	Timeout     int    `yaml:"timeout"`     // timeout in seconds
	DataCenter  string `yaml:"datacenter"`  // data center identifier
	Rack        string `yaml:"rack"`        // rack identifier
}

// SeaweedFSClient implements StorageProvider for SeaweedFS
type SeaweedFSClient struct {
	masterURL   string
	filerURL    string
	collection  string
	replication string
	timeout     time.Duration
	httpClient  *http.Client
}

// Ensure SeaweedFSClient implements StorageProvider
var _ StorageProvider = (*SeaweedFSClient)(nil)

// NewSeaweedFSClient creates a new SeaweedFS storage client
func NewSeaweedFSClient(cfg SeaweedFSConfig) (*SeaweedFSClient, error) {
	if cfg.Master == "" {
		return nil, fmt.Errorf("seaweedfs master address is required")
	}
	if cfg.Filer == "" {
		return nil, fmt.Errorf("seaweedfs filer address is required")
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	collection := cfg.Collection
	if collection == "" {
		collection = "ploy-artifacts"
	}

	replication := cfg.Replication
	if replication == "" {
		replication = "001" // default replication
	}

	client := &SeaweedFSClient{
		masterURL:   ensureHTTPScheme(cfg.Master),
		filerURL:    ensureHTTPScheme(cfg.Filer),
		collection:  collection,
		replication: replication,
		timeout:     timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}

	return client, nil
}

func ensureHTTPScheme(addr string) string {
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		return "http://" + addr
	}
	return addr
}

// StorageProvider interface implementation for SeaweedFS

func (c *SeaweedFSClient) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error) {
	// Create directory structure in filer if needed
	dir := filepath.Dir(key)
	if dir != "." && dir != "/" {
		if err := c.createDirectory(bucket, dir); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Use filer's direct upload endpoint which handles both volume upload and registration
	url := fmt.Sprintf("%s/%s/%s?collection=%s&replication=%s", c.filerURL, bucket, key, c.collection, c.replication)
	
	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	
	// Add file field
	fileWriter, err := writer.CreateFormFile("file", filepath.Base(key))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	
	_, err = io.Copy(fileWriter, body)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file data: %w", err)
	}
	
	writer.Close()
	
	// Make the request
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to upload: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed: %s", resp.Status)
	}
	
	// Parse response to get file info
	var result struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	return &PutObjectResult{
		ETag:     "", // Not provided by filer direct upload
		Location: fmt.Sprintf("%s/%s", bucket, key),
		Size:     result.Size,
	}, nil
}

func (c *SeaweedFSClient) GetObject(bucket, key string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, bucket, key)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to get object: %s", resp.Status)
	}
	return resp.Body, nil
}

func (c *SeaweedFSClient) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, bucket, prefix)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Directory doesn't exist yet, return empty list
		return []ObjectInfo{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list objects: %s", resp.Status)
	}

	var result struct {
		Entries []struct {
			FullPath string `json:"FullPath"`
			FileSize int64  `json:"FileSize"`
			Mode     int64  `json:"Mode"`
			Mtime    string `json:"Mtime"`
		} `json:"Entries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var objects []ObjectInfo
	for _, entry := range result.Entries {
		// Extract just the name from the full path
		name := filepath.Base(entry.FullPath)
		// Include both files and directories for listing
		objects = append(objects, ObjectInfo{
			Key:          name,
			Size:         entry.FileSize,
			LastModified: entry.Mtime,
			ContentType:  "application/octet-stream", // Default, SeaweedFS doesn't store this in listing
		})
	}

	return objects, nil
}

func (c *SeaweedFSClient) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	// Upload main artifact
	if err := c.uploadFile(keyPrefix, artifactPath, "application/octet-stream"); err != nil {
		return fmt.Errorf("failed to upload artifact %s: %w", artifactPath, err)
	}

	// Upload SBOM if exists
	sbomPath := artifactPath + ".sbom.json"
	if _, err := os.Stat(sbomPath); err == nil {
		if err := c.uploadFile(keyPrefix, sbomPath, "application/json"); err != nil {
			return fmt.Errorf("failed to upload SBOM %s: %w", sbomPath, err)
		}
	}

	// Upload signature if exists
	sigPath := artifactPath + ".sig"
	if _, err := os.Stat(sigPath); err == nil {
		if err := c.uploadFile(keyPrefix, sigPath, "application/octet-stream"); err != nil {
			return fmt.Errorf("failed to upload signature %s: %w", sigPath, err)
		}
	}

	// Upload certificate if exists (from keyless OIDC signing)
	crtPath := artifactPath + ".crt"
	if _, err := os.Stat(crtPath); err == nil {
		if err := c.uploadFile(keyPrefix, crtPath, "application/x-pem-file"); err != nil {
			return fmt.Errorf("failed to upload certificate %s: %w", crtPath, err)
		}
	}

	return nil
}

func (c *SeaweedFSClient) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*BundleIntegrityResult, error) {
	// First upload the bundle using existing method
	if err := c.UploadArtifactBundle(keyPrefix, artifactPath); err != nil {
		return nil, fmt.Errorf("failed to upload artifact bundle: %w", err)
	}

	// Then perform integrity verification
	verifier := NewIntegrityVerifier(c)
	result, err := verifier.VerifyArtifactBundle(keyPrefix, artifactPath)
	if err != nil {
		return result, fmt.Errorf("integrity verification failed: %w", err)
	}

	return result, nil
}

func (c *SeaweedFSClient) VerifyUpload(key string) error {
	// Use HEAD request to check if file exists
	url := fmt.Sprintf("%s/%s", c.filerURL, key)
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("object not found: %s", resp.Status)
	}
	return nil
}

func (c *SeaweedFSClient) GetProviderType() string {
	return "seaweedfs"
}

func (c *SeaweedFSClient) GetArtifactsBucket() string {
	return c.collection
}

// TestVolumeAssignment tests if volume assignment is working without actually uploading
func (c *SeaweedFSClient) TestVolumeAssignment() (map[string]interface{}, error) {
	assignment, err := c.assignVolume()
	if err != nil {
		return nil, err
	}
	
	// Return assignment details for health check
	return map[string]interface{}{
		"fid":       assignment.FileID,
		"url":       assignment.URL,
		"publicUrl": assignment.PublicURL,
		"count":     assignment.Count,
	}, nil
}

// SeaweedFS-specific helper methods

type VolumeAssignment struct {
	FileID    string `json:"fid"`
	URL       string `json:"url"`
	PublicURL string `json:"publicUrl"`
	Count     int    `json:"count"`
}

func (c *SeaweedFSClient) assignVolume() (*VolumeAssignment, error) {
	url := fmt.Sprintf("%s/dir/assign?collection=%s&replication=%s", c.masterURL, c.collection, c.replication)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to assign volume: %s", resp.Status)
	}

	var assignment VolumeAssignment
	if err := json.NewDecoder(resp.Body).Decode(&assignment); err != nil {
		return nil, err
	}

	return &assignment, nil
}

func (c *SeaweedFSClient) uploadToVolume(assignment *VolumeAssignment, body io.ReadSeeker, contentType string) (string, int64, error) {
	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file part
	part, err := writer.CreateFormFile("file", "artifact")
	if err != nil {
		return "", 0, err
	}

	size, err := io.Copy(part, body)
	if err != nil {
		return "", 0, err
	}

	writer.Close()

	// Upload to volume server
	uploadURL := fmt.Sprintf("http://%s/%s", assignment.URL, assignment.FileID)
	req, err := http.NewRequest("POST", uploadURL, &buf)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("upload failed: %s", resp.Status)
	}

	return assignment.FileID, size, nil
}

func (c *SeaweedFSClient) createDirectory(bucket, dir string) error {
	url := fmt.Sprintf("%s/%s/%s/", c.filerURL, bucket, dir)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}
	
	// Don't set Content-Type header - SeaweedFS doesn't like it for directory creation
	// req.Header.Set("Content-Type", "application/x-directory")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Accept 409 Conflict as success (directory already exists)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("failed to create directory: %s", resp.Status)
	}

	return nil
}

// registerInFiler registers an already uploaded file (by fileID) in the filer
// This is used for advanced scenarios where files are uploaded directly to volume servers
// and need to be registered in the filer separately. For normal use cases, use PutObject
// which uses the filer's direct upload API that handles both upload and registration.
func (c *SeaweedFSClient) registerInFiler(bucket, key, fileID, contentType string, size int64) error {
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, bucket, key)

	// Create multipart form data with file metadata
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	
	// Add metadata fields
	writer.WriteField("fid", fileID)
	writer.WriteField("size", fmt.Sprintf("%d", size))
	if contentType != "" {
		writer.WriteField("mime", contentType)
	}
	
	writer.Close()

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to register in filer: %s", resp.Status)
	}

	return nil
}

func (c *SeaweedFSClient) uploadFile(keyPrefix, filePath, contentType string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	key := keyPrefix + filepath.Base(filePath)

	// Retry upload up to 3 times
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		// Reset file pointer to beginning
		if _, err := file.Seek(0, 0); err != nil {
			return fmt.Errorf("failed to reset file pointer: %w", err)
		}

		result, err := c.PutObject(c.collection, key, file, contentType)
		if err != nil {
			lastErr = err
			fmt.Printf("SeaweedFS upload attempt %d failed for %s: %v\n", attempt, key, err)
			continue
		}

		// Verify upload by checking if we got a valid result with size
		if result != nil && result.Size > 0 {
			fmt.Printf("Successfully uploaded to SeaweedFS %s (size: %d bytes)\n", key, result.Size)
			return nil
		}

		lastErr = fmt.Errorf("upload completed but no valid result received")
	}

	return fmt.Errorf("failed to upload after 3 attempts: %w", lastErr)
}
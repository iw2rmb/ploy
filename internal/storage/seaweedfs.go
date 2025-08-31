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
		collection = "artifacts"
	}

	replication := cfg.Replication
	if replication == "" {
		replication = "000" // no replication for dev environment
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
	fmt.Printf("[SeaweedFS PutObject] Starting upload - bucket: %s, key: %s, contentType: %s\n", bucket, key, contentType)

	// Create directory structure in filer if needed
	dir := filepath.Dir(key)
	fmt.Printf("[SeaweedFS PutObject] Directory path: %s\n", dir)
	if dir != "." && dir != "/" {
		fmt.Printf("[SeaweedFS PutObject] Creating directory: %s/%s\n", bucket, dir)
		if err := c.createDirectory(bucket, dir); err != nil {
			fmt.Printf("[SeaweedFS PutObject] ERROR: Failed to create directory %s/%s: %v\n", bucket, dir, err)
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
		fmt.Printf("[SeaweedFS PutObject] Directory created successfully\n")
	}

	// Use filer's direct upload endpoint which handles both volume upload and registration
	// Note: We don't include collection parameter as it causes "context canceled" errors in SeaweedFS
	url := fmt.Sprintf("%s/%s/%s?replication=%s", c.filerURL, bucket, key, c.replication)
	fmt.Printf("[SeaweedFS PutObject] Upload URL: %s\n", url)

	// Get file size for logging
	fileSize := int64(0)
	if size, err := body.Seek(0, 2); err == nil { // Seek to end to get size
		fileSize = size
	}
	body.Seek(0, 0) // Reset to start for actual upload
	fmt.Printf("[SeaweedFS PutObject] File size: %d bytes\n", fileSize)

	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file field
	fileWriter, err := writer.CreateFormFile("file", filepath.Base(key))
	if err != nil {
		fmt.Printf("[SeaweedFS PutObject] ERROR: Failed to create form file: %v\n", err)
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	bytesWritten, err := io.Copy(fileWriter, body)
	if err != nil {
		fmt.Printf("[SeaweedFS PutObject] ERROR: Failed to copy file data: %v\n", err)
		return nil, fmt.Errorf("failed to copy file data: %w", err)
	}
	fmt.Printf("[SeaweedFS PutObject] Copied %d bytes to multipart form\n", bytesWritten)

	writer.Close()
	fmt.Printf("[SeaweedFS PutObject] Multipart form size: %d bytes\n", buf.Len())

	// Make the request
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		fmt.Printf("[SeaweedFS PutObject] ERROR: Failed to create request: %v\n", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	fmt.Printf("[SeaweedFS PutObject] Request Content-Type: %s\n", writer.FormDataContentType())

	fmt.Printf("[SeaweedFS PutObject] Executing HTTP POST request...\n")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Printf("[SeaweedFS PutObject] ERROR: HTTP request failed: %v\n", err)
		return nil, fmt.Errorf("failed to upload: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("[SeaweedFS PutObject] HTTP Response Status: %s (%d)\n", resp.Status, resp.StatusCode)

	// Read response body for debugging
	responseBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("[SeaweedFS PutObject] Response Body: %s\n", string(responseBody))

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		fmt.Printf("[SeaweedFS PutObject] ERROR: Upload failed with status %s, body: %s\n", resp.Status, string(responseBody))
		return nil, fmt.Errorf("upload failed: %s", resp.Status)
	}

	// Parse response to get file info
	var result struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}

	// Reset response body reader since we already read it
	responseReader := bytes.NewReader(responseBody)
	if err := json.NewDecoder(responseReader).Decode(&result); err != nil {
		fmt.Printf("[SeaweedFS PutObject] ERROR: Failed to parse response JSON: %v\n", err)
		fmt.Printf("[SeaweedFS PutObject] Raw response: %s\n", string(responseBody))
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("[SeaweedFS PutObject] Upload successful - Name: %s, Size: %d\n", result.Name, result.Size)

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
	// Use HEAD request to check if file exists - use the same bucket pattern as PutObject
	// NOTE: We use the artifacts bucket (collection) to match PutObject behavior
	bucket := c.GetArtifactsBucket()
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, bucket, key)
	fmt.Printf("[SeaweedFS VerifyUpload] Checking upload at URL: %s (bucket: %s)\n", url, bucket)

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		fmt.Printf("[SeaweedFS VerifyUpload] ERROR: Failed to create HEAD request: %v\n", err)
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Printf("[SeaweedFS VerifyUpload] ERROR: HEAD request failed: %v\n", err)
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("[SeaweedFS VerifyUpload] HEAD response status: %s (%d)\n", resp.Status, resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[SeaweedFS VerifyUpload] ERROR: Object not found at %s, status: %s\n", url, resp.Status)
		return fmt.Errorf("object not found: %s", resp.Status)
	}

	fmt.Printf("[SeaweedFS VerifyUpload] SUCCESS: Upload verified at %s\n", url)
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

	// Read response body for debugging
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[SeaweedFS createDirectory] Response Status: %d, Body: %s\n", resp.StatusCode, string(body))

	// Accept 409 Conflict as success (directory already exists)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("failed to create directory: %s, body: %s", resp.Status, string(body))
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

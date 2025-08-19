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
	// Get volume assignment from master
	assignment, err := c.assignVolume()
	if err != nil {
		return nil, fmt.Errorf("failed to assign volume: %w", err)
	}

	// Upload to volume server
	fileID, size, err := c.uploadToVolume(assignment, body, contentType)
	if err != nil {
		return nil, fmt.Errorf("failed to upload to volume: %w", err)
	}

	// Create directory structure in filer if needed
	dir := filepath.Dir(key)
	if dir != "." && dir != "/" {
		if err := c.createDirectory(bucket, dir); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Register file in filer
	if err := c.registerInFiler(bucket, key, fileID, contentType, size); err != nil {
		return nil, fmt.Errorf("failed to register in filer: %w", err)
	}

	return &PutObjectResult{
		ETag:     fileID,
		Location: fmt.Sprintf("%s/%s", bucket, key),
		Size:     size,
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
	url := fmt.Sprintf("%s/%s/%s?pretty=y", c.filerURL, bucket, prefix)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list objects: %s", resp.Status)
	}

	var result struct {
		Entries []struct {
			Name  string `json:"Name"`
			Size  int64  `json:"Size"`
			Mode  int64  `json:"Mode"`
			Mtime string `json:"Mtime"`
		} `json:"Entries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var objects []ObjectInfo
	for _, entry := range result.Entries {
		// Skip directories (Mode & os.ModeDir != 0)
		if entry.Mode&(1<<31) == 0 { // Not a directory
			objects = append(objects, ObjectInfo{
				Key:          prefix + "/" + entry.Name,
				Size:         entry.Size,
				LastModified: entry.Mtime,
				ContentType:  "application/octet-stream", // Default, SeaweedFS doesn't store this in listing
			})
		}
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to create directory: %s", resp.Status)
	}

	return nil
}

func (c *SeaweedFSClient) registerInFiler(bucket, key, fileID, contentType string, size int64) error {
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, bucket, key)

	// Create the file metadata
	metadata := map[string]interface{}{
		"fid":  fileID,
		"size": size,
		"mime": contentType,
	}

	jsonData, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

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

	// Get file info for verification
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

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

		// Verify upload by checking if we got a file ID
		if result.ETag != "" {
			fmt.Printf("Successfully uploaded to SeaweedFS %s (size: %d bytes, FileID: %s)\n", key, fileInfo.Size(), result.ETag)
			return nil
		}

		lastErr = fmt.Errorf("upload completed but no file ID received")
	}

	return fmt.Errorf("failed to upload after 3 attempts: %w", lastErr)
}
package seaweedfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// Provider implements both storage.Storage and storage.StorageProvider interfaces for SeaweedFS
type Provider struct {
	masterURL   string
	filerURL    string
	collection  string
	replication string
	timeout     time.Duration
	httpClient  *http.Client
}

// Ensure Provider implements both interfaces
var _ storage.Storage = (*Provider)(nil)
var _ storage.StorageProvider = (*Provider)(nil)

// New creates a new SeaweedFS storage provider
func New(cfg Config) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	timeout := cfg.TimeoutDuration()
	collection := cfg.Collection
	if collection == "" {
		collection = "artifacts"
	}

	replication := cfg.Replication
	if replication == "" {
		replication = "000" // no replication for dev environment
	}

	provider := &Provider{
		masterURL:   ensureHTTPScheme(cfg.Master),
		filerURL:    ensureHTTPScheme(cfg.Filer),
		collection:  collection,
		replication: replication,
		timeout:     timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}

	return provider, nil
}

func ensureHTTPScheme(addr string) string {
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		return "http://" + addr
	}
	return addr
}

// Storage interface implementation

func (p *Provider) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	// Use collection as bucket for Storage interface
	return p.GetObject(p.collection, key)
}

func (p *Provider) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	// Minimal implementation for GREEN phase - use default content type
	// Options parsing will be implemented properly in the REFACTOR phase
	contentType := "application/octet-stream"

	// Convert io.Reader to io.ReadSeeker (minimal approach for now)
	var readSeeker io.ReadSeeker
	if rs, ok := reader.(io.ReadSeeker); ok {
		readSeeker = rs
	} else {
		// Read into memory for now - proper streaming would be implemented later
		data, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("failed to read data: %w", err)
		}
		readSeeker = bytes.NewReader(data)
	}

	// Use collection as bucket and delegate to StorageProvider method
	_, err := p.PutObject(p.collection, key, readSeeker, contentType)
	return err
}

func (p *Provider) Delete(ctx context.Context, key string) error {
	// Minimal implementation - SeaweedFS delete support
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, p.collection, key)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete failed: %s", resp.Status)
	}

	return nil
}

func (p *Provider) Exists(ctx context.Context, key string) (bool, error) {
	// Use VerifyUpload method as it checks existence
	err := p.VerifyUpload(key)
	if err != nil {
		return false, nil // Assume doesn't exist if verification fails
	}
	return true, nil
}

func (p *Provider) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	// Use existing ListObjects method and convert ObjectInfo to Object
	objectInfos, err := p.ListObjects(p.collection, opts.Prefix)
	if err != nil {
		return nil, err
	}

	objects := make([]storage.Object, len(objectInfos))
	for i, info := range objectInfos {
		// Parse time if possible, use zero time otherwise
		var lastModified time.Time
		if info.LastModified != "" {
			if parsed, err := time.Parse(time.RFC3339, info.LastModified); err == nil {
				lastModified = parsed
			}
		}

		objects[i] = storage.Object{
			Key:          info.Key,
			Size:         info.Size,
			ContentType:  info.ContentType,
			ETag:         info.ETag,
			LastModified: lastModified,
			Metadata:     make(map[string]string),
		}
	}

	return objects, nil
}

func (p *Provider) DeleteBatch(ctx context.Context, keys []string) error {
	// Delete each key individually
	for _, key := range keys {
		if err := p.Delete(ctx, key); err != nil {
			return fmt.Errorf("failed to delete key %s: %w", key, err)
		}
	}
	return nil
}

func (p *Provider) Head(ctx context.Context, key string) (*storage.Object, error) {
	// Minimal implementation using HEAD request
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, p.collection, key)
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, storage.NewStorageError("head", fmt.Errorf("object not found"), storage.ErrorContext{Key: key})
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("head request failed: %s", resp.Status)
	}

	// Extract metadata from headers
	contentLength := resp.ContentLength
	contentType := resp.Header.Get("Content-Type")
	etag := resp.Header.Get("ETag")
	lastModifiedStr := resp.Header.Get("Last-Modified")

	var lastModified time.Time
	if lastModifiedStr != "" {
		if parsed, err := time.Parse(http.TimeFormat, lastModifiedStr); err == nil {
			lastModified = parsed
		}
	}

	return &storage.Object{
		Key:          key,
		Size:         contentLength,
		ContentType:  contentType,
		ETag:         etag,
		LastModified: lastModified,
		Metadata:     make(map[string]string),
	}, nil
}

func (p *Provider) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	// SeaweedFS doesn't have native metadata support - this is a minimal stub
	return fmt.Errorf("metadata operations not supported by SeaweedFS")
}

func (p *Provider) Copy(ctx context.Context, src, dst string) error {
	// SeaweedFS doesn't have native copy - implement via get/put
	reader, err := p.Get(ctx, src)
	if err != nil {
		return fmt.Errorf("failed to read source: %w", err)
	}
	defer reader.Close()

	return p.Put(ctx, dst, reader)
}

func (p *Provider) Move(ctx context.Context, src, dst string) error {
	// Implement as copy + delete
	if err := p.Copy(ctx, src, dst); err != nil {
		return err
	}
	return p.Delete(ctx, src)
}

func (p *Provider) Health(ctx context.Context) error {
	// Test volume assignment to check health
	_, err := p.TestVolumeAssignment()
	if err != nil {
		return fmt.Errorf("seaweedfs health check failed: %w", err)
	}
	return nil
}

func (p *Provider) Metrics() *storage.StorageMetrics {
	// Return basic metrics - proper implementation would track operations
	return storage.NewStorageMetrics()
}

// StorageProvider interface implementation (for backward compatibility)

func (p *Provider) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*storage.PutObjectResult, error) {
	fmt.Printf("[SeaweedFS PutObject] Starting upload - bucket: %s, key: %s, contentType: %s\n", bucket, key, contentType)

	// Create directory structure in filer if needed
	dir := filepath.Dir(key)
	fmt.Printf("[SeaweedFS PutObject] Directory path: %s\n", dir)
	if dir != "." && dir != "/" {
		// For unified storage, the key already contains the full path including bucket
		// So we don't need to add bucket prefix again
		fmt.Printf("[SeaweedFS PutObject] Creating directory with full path: %s\n", dir)
		if err := p.createDirectoryFullPath(dir); err != nil {
			fmt.Printf("[SeaweedFS PutObject] ERROR: Failed to create directory %s: %v\n", dir, err)
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
		fmt.Printf("[SeaweedFS PutObject] Directory created successfully\n")
	}

	// Use filer's direct upload endpoint
	// For unified storage, key already contains the full path including bucket
	url := fmt.Sprintf("%s/%s?replication=%s", p.filerURL, key, p.replication)
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
	resp, err := p.httpClient.Do(req)
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

	return &storage.PutObjectResult{
		ETag:     "", // Not provided by filer direct upload
		Location: fmt.Sprintf("%s/%s", bucket, key),
		Size:     result.Size,
	}, nil
}

func (p *Provider) GetObject(bucket, key string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, bucket, key)
	resp, err := p.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to get object: %s", resp.Status)
	}
	return resp.Body, nil
}

func (p *Provider) ListObjects(bucket, prefix string) ([]storage.ObjectInfo, error) {
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, bucket, prefix)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Directory doesn't exist yet, return empty list
		return []storage.ObjectInfo{}, nil
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

	var objects []storage.ObjectInfo
	for _, entry := range result.Entries {
		// Extract just the name from the full path
		name := filepath.Base(entry.FullPath)
		// Include both files and directories for listing
		objects = append(objects, storage.ObjectInfo{
			Key:          name,
			Size:         entry.FileSize,
			LastModified: entry.Mtime,
			ContentType:  "application/octet-stream", // Default, SeaweedFS doesn't store this in listing
		})
	}

	return objects, nil
}

func (p *Provider) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	// Upload main artifact
	if err := p.uploadFile(keyPrefix, artifactPath, "application/octet-stream"); err != nil {
		return fmt.Errorf("failed to upload artifact %s: %w", artifactPath, err)
	}

	// Upload SBOM if exists
	sbomPath := artifactPath + ".sbom.json"
	if _, err := os.Stat(sbomPath); err == nil {
		if err := p.uploadFile(keyPrefix, sbomPath, "application/json"); err != nil {
			return fmt.Errorf("failed to upload SBOM %s: %w", sbomPath, err)
		}
	}

	// Upload signature if exists
	sigPath := artifactPath + ".sig"
	if _, err := os.Stat(sigPath); err == nil {
		if err := p.uploadFile(keyPrefix, sigPath, "application/octet-stream"); err != nil {
			return fmt.Errorf("failed to upload signature %s: %w", sigPath, err)
		}
	}

	// Upload certificate if exists (from keyless OIDC signing)
	crtPath := artifactPath + ".crt"
	if _, err := os.Stat(crtPath); err == nil {
		if err := p.uploadFile(keyPrefix, crtPath, "application/x-pem-file"); err != nil {
			return fmt.Errorf("failed to upload certificate %s: %w", crtPath, err)
		}
	}

	return nil
}

func (p *Provider) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*storage.BundleIntegrityResult, error) {
	// First upload the bundle using existing method
	if err := p.UploadArtifactBundle(keyPrefix, artifactPath); err != nil {
		return nil, fmt.Errorf("failed to upload artifact bundle: %w", err)
	}

	// Then perform integrity verification
	verifier := storage.NewIntegrityVerifier(p)
	result, err := verifier.VerifyArtifactBundle(keyPrefix, artifactPath)
	if err != nil {
		return result, fmt.Errorf("integrity verification failed: %w", err)
	}

	return result, nil
}

func (p *Provider) VerifyUpload(key string) error {
	// Use HEAD request to check if file exists - use the same bucket pattern as PutObject
	bucket := p.GetArtifactsBucket()
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, bucket, key)
	fmt.Printf("[SeaweedFS VerifyUpload] Checking upload at URL: %s (bucket: %s)\n", url, bucket)

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		fmt.Printf("[SeaweedFS VerifyUpload] ERROR: Failed to create HEAD request: %v\n", err)
		return err
	}

	resp, err := p.httpClient.Do(req)
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

func (p *Provider) GetProviderType() string {
	return "seaweedfs"
}

func (p *Provider) GetArtifactsBucket() string {
	return p.collection
}

// TestVolumeAssignment tests if volume assignment is working without actually uploading
func (p *Provider) TestVolumeAssignment() (map[string]interface{}, error) {
	assignment, err := p.assignVolume()
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

func (p *Provider) assignVolume() (*VolumeAssignment, error) {
	url := fmt.Sprintf("%s/dir/assign?collection=%s&replication=%s", p.masterURL, p.collection, p.replication)
	resp, err := p.httpClient.Get(url)
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

func (p *Provider) createDirectory(bucket, dir string) error {
	url := fmt.Sprintf("%s/%s/%s/", p.filerURL, bucket, dir)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
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

func (p *Provider) createDirectoryFullPath(fullPath string) error {
	// For unified storage where the path already includes all components
	url := fmt.Sprintf("%s/%s/", p.filerURL, fullPath)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read response body for debugging
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[SeaweedFS createDirectoryFullPath] Response Status: %d, Body: %s\n", resp.StatusCode, string(body))

	// Accept 409 Conflict as success (directory already exists)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("failed to create directory: %s, body: %s", resp.Status, string(body))
	}

	return nil
}

func (p *Provider) uploadFile(keyPrefix, filePath, contentType string) error {
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

		result, err := p.PutObject(p.collection, key, file, contentType)
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

package seaweedfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/iw2rmb/ploy/internal/storage"
)

// StorageProvider interface implementation (for backward compatibility)

func (p *Provider) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*storage.PutObjectResult, error) {
	log.Printf("[SeaweedFS PutObject] Starting upload - bucket: %s, key: %s, contentType: %s", bucket, key, contentType)
	var fullPath string
	if bucket != "" {
		fullPath = fmt.Sprintf("%s/%s", bucket, key)
	} else {
		fullPath = key
	}
	dir := filepath.Dir(fullPath)
	if dir != "." && dir != "/" {
		if err := p.createDirectoryFullPath(dir); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}
	url := fmt.Sprintf("%s/%s?replication=%s", p.filerURL, fullPath, p.replication)
	// Reset and get size
	fileSize := int64(0)
	if size, err := body.Seek(0, 2); err == nil {
		fileSize = size
	}
	if _, err := body.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to reset body: %w", err)
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	fileWriter, err := writer.CreateFormFile("file", filepath.Base(key))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(fileWriter, body); err != nil {
		return nil, fmt.Errorf("failed to copy file data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to upload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed: %s", resp.Status)
	}
	var result struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if err := json.NewDecoder(bytes.NewReader(responseBody)).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	_ = fileSize // retained for potential logging
	return &storage.PutObjectResult{ETag: "", Location: fmt.Sprintf("%s/%s", bucket, key), Size: result.Size}, nil
}

func (p *Provider) GetObject(bucket, key string) (io.ReadCloser, error) {
	var fullPath string
	if bucket != "" {
		fullPath = fmt.Sprintf("%s/%s", bucket, key)
	} else {
		fullPath = key
	}
	url := fmt.Sprintf("%s/%s", p.filerURL, fullPath)
	resp, err := p.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("get object failed: %s", resp.Status)
	}
	return resp.Body, nil
}

func (p *Provider) ListObjects(bucket, prefix string) ([]storage.ObjectInfo, error) {
	var fullPath string
	if bucket != "" {
		fullPath = fmt.Sprintf("%s/%s", bucket, prefix)
	} else {
		fullPath = prefix
	}
	url := fmt.Sprintf("%s/%s", p.filerURL, fullPath)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
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
		name := filepath.Base(entry.FullPath)
		objects = append(objects, storage.ObjectInfo{Key: name, Size: entry.FileSize, LastModified: entry.Mtime, ContentType: "application/octet-stream"})
	}
	return objects, nil
}

func (p *Provider) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	if err := p.uploadFile(keyPrefix, artifactPath, "application/octet-stream"); err != nil {
		return fmt.Errorf("failed to upload artifact %s: %w", artifactPath, err)
	}
	sbomPath := artifactPath + ".sbom.json"
	if _, err := os.Stat(sbomPath); err == nil {
		if err := p.uploadFile(keyPrefix, sbomPath, "application/json"); err != nil {
			return fmt.Errorf("failed to upload SBOM %s: %w", sbomPath, err)
		}
	}
	sigPath := artifactPath + ".sig"
	if _, err := os.Stat(sigPath); err == nil {
		if err := p.uploadFile(keyPrefix, sigPath, "application/octet-stream"); err != nil {
			return fmt.Errorf("failed to upload signature %s: %w", sigPath, err)
		}
	}
	crtPath := artifactPath + ".crt"
	if _, err := os.Stat(crtPath); err == nil {
		if err := p.uploadFile(keyPrefix, crtPath, "application/x-pem-file"); err != nil {
			return fmt.Errorf("failed to upload certificate %s: %w", crtPath, err)
		}
	}
	return nil
}

func (p *Provider) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*storage.BundleIntegrityResult, error) {
	if err := p.UploadArtifactBundle(keyPrefix, artifactPath); err != nil {
		return nil, fmt.Errorf("failed to upload artifact bundle: %w", err)
	}
	verifier := storage.NewIntegrityVerifier(p)
	result, err := verifier.VerifyArtifactBundle(keyPrefix, artifactPath)
	if err != nil {
		return result, fmt.Errorf("integrity verification failed: %w", err)
	}
	return result, nil
}

func (p *Provider) VerifyUpload(key string) error {
	bucket := p.GetArtifactsBucket()
	url := fmt.Sprintf("%s/%s/%s", p.filerURL, bucket, key)
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("object not found: %s", resp.Status)
	}
	return nil
}

func (p *Provider) GetProviderType() string    { return "seaweedfs" }
func (p *Provider) GetArtifactsBucket() string { return p.collection }

// TestVolumeAssignment tests if volume assignment is working without actually uploading
func (p *Provider) TestVolumeAssignment() (map[string]interface{}, error) {
	assignment, err := p.assignVolume()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"fid": assignment.FileID, "url": assignment.URL, "publicUrl": assignment.PublicURL, "count": assignment.Count}, nil
}

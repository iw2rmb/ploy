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
)

func (c *SeaweedFSClient) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error) {
	fmt.Printf("[SeaweedFS PutObject] Starting upload - bucket: %s, key: %s, contentType: %s\n", bucket, key, contentType)

	dir := filepath.Dir(key)
	fmt.Printf("[SeaweedFS PutObject] Directory path: %s\n", dir)
	if dir != "." && dir != "/" {
		fmt.Printf("[SeaweedFS PutObject] Creating directory with full path: %s\n", dir)
		if err := c.createDirectoryFullPath(dir); err != nil {
			fmt.Printf("[SeaweedFS PutObject] ERROR: Failed to create directory %s: %v\n", dir, err)
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
		fmt.Printf("[SeaweedFS PutObject] Directory created successfully\n")
	}

	url := fmt.Sprintf("%s/%s?replication=%s", c.filerURL, key, c.replication)
	fmt.Printf("[SeaweedFS PutObject] Upload URL: %s\n", url)

	fileSize := int64(0)
	if size, err := body.Seek(0, io.SeekEnd); err == nil {
		fileSize = size
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to reset body: %w", err)
	}
	fmt.Printf("[SeaweedFS PutObject] File size: %d bytes\n", fileSize)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

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

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}
	fmt.Printf("[SeaweedFS PutObject] Multipart form size: %d bytes\n", buf.Len())

	req, err := http.NewRequest(http.MethodPost, url, &buf)
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
	defer func() { _ = resp.Body.Close() }()

	fmt.Printf("[SeaweedFS PutObject] HTTP Response Status: %s (%d)\n", resp.Status, resp.StatusCode)

	responseBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("[SeaweedFS PutObject] Response Body: %s\n", string(responseBody))

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		fmt.Printf("[SeaweedFS PutObject] ERROR: Upload failed with status %s, body: %s\n", resp.Status, string(responseBody))
		return nil, fmt.Errorf("upload failed: %s", resp.Status)
	}

	var result struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if err := json.NewDecoder(bytes.NewReader(responseBody)).Decode(&result); err != nil {
		fmt.Printf("[SeaweedFS PutObject] ERROR: Failed to parse response JSON: %v\n", err)
		fmt.Printf("[SeaweedFS PutObject] Raw response: %s\n", string(responseBody))
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("[SeaweedFS PutObject] Upload successful - Name: %s, Size: %d\n", result.Name, result.Size)

	return &PutObjectResult{
		ETag:     "",
		Location: fmt.Sprintf("%s/%s", bucket, key),
		Size:     result.Size,
	}, nil
}

func (c *SeaweedFSClient) createDirectory(bucket, dir string) error {
	url := fmt.Sprintf("%s/%s/%s/", c.filerURL, bucket, dir)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[SeaweedFS createDirectory] Response Status: %d, Body: %s\n", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("failed to create directory: %s, body: %s", resp.Status, string(body))
	}

	return nil
}

func (c *SeaweedFSClient) createDirectoryFullPath(fullPath string) error {
	url := fmt.Sprintf("%s/%s/", c.filerURL, fullPath)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[SeaweedFS createDirectoryFullPath] Response Status: %d, Body: %s\n", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("failed to create directory: %s, body: %s", resp.Status, string(body))
	}

	return nil
}

func (c *SeaweedFSClient) uploadToVolume(assignment *VolumeAssignment, body io.ReadSeeker, contentType string) (string, int64, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "artifact")
	if err != nil {
		return "", 0, err
	}

	size, err := io.Copy(part, body)
	if err != nil {
		return "", 0, err
	}

	_ = writer.Close()

	uploadURL := fmt.Sprintf("http://%s/%s", assignment.URL, assignment.FileID)
	req, err := http.NewRequest(http.MethodPost, uploadURL, &buf)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("upload failed: %s", resp.Status)
	}

	return assignment.FileID, size, nil
}

func (c *SeaweedFSClient) registerInFiler(bucket, key, fileID, contentType string, size int64) error {
	url := fmt.Sprintf("%s/%s/%s", c.filerURL, bucket, key)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("fid", fileID); err != nil {
		return err
	}
	if err := writer.WriteField("size", fmt.Sprintf("%d", size)); err != nil {
		return err
	}
	if contentType != "" {
		if err := writer.WriteField("mime", contentType); err != nil {
			return err
		}
	}

	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = file.Close() }()

	key := keyPrefix + filepath.Base(filePath)

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("failed to reset file pointer: %w", err)
		}

		result, err := c.PutObject(c.collection, key, file, contentType)
		if err != nil {
			lastErr = err
			fmt.Printf("SeaweedFS upload attempt %d failed for %s: %v\n", attempt, key, err)
			continue
		}

		if result != nil && result.Size > 0 {
			fmt.Printf("Successfully uploaded to SeaweedFS %s (size: %d bytes)\n", key, result.Size)
			return nil
		}

		lastErr = fmt.Errorf("upload completed but no valid result received")
	}

	return fmt.Errorf("failed to upload after 3 attempts: %w", lastErr)
}
